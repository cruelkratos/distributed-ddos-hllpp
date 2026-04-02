// Agent runs the DDoS detection pipeline: packet capture -> HLL window -> detector -> metrics/alerts.
// Supports ensemble ML detection, state machine, mitigation, and simulation mode.
package main

import (
	"HLL-BTP/ddos/capture"
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/metrics"
	"HLL-BTP/ddos/mitigation"
	"HLL-BTP/ddos/window"
	pb "HLL-BTP/server"
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	iface := flag.String("iface", "", "Network interface for capture (e.g. eth0). Empty for default.")
	windowDur := flag.Duration("window", 10*time.Second, "Time window for distinct-IP count.")
	threshold := flag.Uint64("threshold", 5000, "Distinct IPs per window above which to signal attack.")
	metricsAddr := flag.String("metrics", ":9090", "HTTP address for /metrics.")
	checkInterval := flag.Duration("check", time.Second, "Interval for detection check and metrics update.")
	ipsBuf := flag.Int("ips-buf", 10000, "Buffer size for IP channel.")
	detectorType := flag.String("detector", "ensemble", "Detector type: threshold, zscore, ensemble")
	aggregatorAddr := flag.String("aggregator", "", "gRPC address of aggregator (e.g. aggregator-service:50051). Empty = standalone.")
	shipInterval := flag.Duration("ship-interval", 0, "Interval for shipping sketches to aggregator. Defaults to window duration.")
	nodeID := flag.String("node-id", "", "Unique node identifier. Defaults to hostname.")
	simMode := flag.Bool("sim-mode", false, "Enable simulation mode (accepts InjectIP gRPC calls instead of pcap).")
	simGRPCAddr := flag.String("sim-grpc", ":50052", "gRPC listen address for simulation mode InjectIP service.")
	ensembleThreshold := flag.Float64("ensemble-threshold", 0.6, "Ensemble anomaly score threshold for attack detection.")
	globalRPS := flag.Float64("global-rps", 1000, "Global rate limit (requests/second) during mitigation.")
	perIPLimit := flag.Uint64("per-ip-limit", 50, "Per-IP rate limit during mitigation.")
	flag.Parse()

	// Allow env override for aggregator address (K8s-friendly).
	if *aggregatorAddr == "" {
		*aggregatorAddr = os.Getenv("AGGREGATOR_ADDR")
	}
	if *shipInterval <= 0 {
		*shipInterval = *windowDur
	}
	if *nodeID == "" {
		h, err := os.Hostname()
		if err != nil {
			h = "unknown"
		}
		*nodeID = h
	}

	// Create detector.
	var det detector.Detector
	var ensemble *detector.EnsembleDetector
	switch *detectorType {
	case "ensemble":
		ensemble = detector.NewEnsembleDetector(42, *ensembleThreshold, detector.DefaultEnsembleWeights())
		det = ensemble
	case "zscore":
		det = detector.NewZScoreDetector(20, 3.0)
	default:
		det = detector.NewThresholdDetector(*threshold)
	}

	// Create state machine and mitigation controller.
	sm := detector.NewAnomalyStateMachine(*ensembleThreshold, 3, 5)
	mc := mitigation.NewMitigationController(sm, *globalRPS, uint16(*perIPLimit))

	attackCh := make(chan window.AttackEvent, 16)
	wm := window.NewWindowManager(*windowDur, *checkInterval, det, attackCh)

	// Goroutine: handle attack events and drive state machine.
	go func() {
		for ev := range attackCh {
			log.Printf("[ATTACK] reason=%s count=%d window_id=%d at=%s state=%s",
				ev.Reason, ev.Count, ev.WindowID, ev.At.Format(time.RFC3339), sm.State())
		}
	}()

	// Initial metrics update.
	metrics.UpdateWindowMetrics(wm.CurrentCount(), false, wm.ApproxMemoryBytes())

	// Goroutine: periodic metrics update and state machine driving.
	go func() {
		ticker := time.NewTicker(*checkInterval)
		defer ticker.Stop()
		for range ticker.C {
			features := wm.LastFeatures()
			cur := wm.CurrentCount()
			features.CurrentWindowCount = cur
			features.PreviousWindowCount = wm.PreviousCount()
			features.WindowDurationSec = (*windowDur).Seconds()

			// Compute ensemble score and drive state machine.
			var ensembleScore, lodaScore, hstScore float64
			attack := det.IsAttack(features)
			if ensemble != nil {
				score := ensemble.Score(features)
				components := ensemble.GetComponents()
				ensembleScore = score
				lodaScore = components.LODAScore
				hstScore = components.HSTScore
				features.EWMAResidual = components.EWMAResidual
				features.ZScoreValue = components.ZScoreValue

				newState := sm.Transition(score)
				mc.UpdateState(newState)
			} else if attack {
				sm.ForceState(detector.StateUnderAttack)
				mc.UpdateState(detector.StateUnderAttack)
			} else {
				newState := sm.Transition(0)
				mc.UpdateState(newState)
			}

			mem := wm.ApproxMemoryBytes()
			metrics.UpdateWindowMetrics(cur, attack, mem)
			metrics.UpdateExtendedMetrics(
				features.PacketCount, features.ByteVolume,
				lodaScore, hstScore, ensembleScore,
				int(sm.State()), mc.DroppedCount(),
			)
		}
	}()

	// IP processing channel.
	ipsChan := make(chan string, *ipsBuf)
	var bytesChan chan uint64

	if *simMode {
		// Simulation mode: accept IPs via gRPC InjectIP service.
		bytesChan = make(chan uint64, *ipsBuf)
		simSrv := &simServer{ipsChan: ipsChan, bytesChan: bytesChan}
		lis, err := net.Listen("tcp", *simGRPCAddr)
		if err != nil {
			log.Fatalf("sim-mode: failed to listen on %s: %v", *simGRPCAddr, err)
		}
		grpcSim := grpc.NewServer()
		pb.RegisterHllServiceServer(grpcSim, simSrv)
		go func() {
			log.Printf("sim-mode: InjectIP gRPC listening on %s", *simGRPCAddr)
			if err := grpcSim.Serve(lis); err != nil {
				log.Printf("sim gRPC serve: %v", err)
			}
		}()
	} else {
		// Production mode: pcap capture.
		ps := capture.NewPcapPacketSource(*iface, 256, false, 100*time.Millisecond)
		go func() {
			if err := ps.Run(ipsChan); err != nil {
				log.Printf("capture stopped: %v", err)
			}
			close(ipsChan)
		}()
		defer ps.Stop()
	}

	// Goroutine: drain IPs into WindowManager (with mitigation gate).
	var insertedCount atomic.Uint64
	go func() {
		for ip := range ipsChan {
			if !mc.ShouldAllow(ip) {
				continue // dropped by rate limiter
			}
			_ = wm.Insert(ip)
			if bytesChan == nil {
				// pcap mode: record stats here (no bytes channel).
				wm.Stats.RecordPacket(100)
			}
			insertedCount.Add(1)
		}
	}()

	// If sim mode also has a bytes channel, drain it to update stats.
	if bytesChan != nil {
		go func() {
			for b := range bytesChan {
				wm.Stats.RecordPacket(b)
			}
		}()
	}

	// Metrics HTTP server.
	go func() {
		if err := metrics.ListenAndServe(*metricsAddr); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	log.Printf("agent started: node_id=%s iface=%q window=%s detector=%s threshold=%d metrics=%s aggregator=%q sim-mode=%v",
		*nodeID, *iface, *windowDur, *detectorType, *threshold, *metricsAddr, *aggregatorAddr, *simMode)

	// gRPC sketch shipping to aggregator (if configured).
	var grpcConn *grpc.ClientConn
	if *aggregatorAddr != "" {
		var err error
		grpcConn, err = grpc.NewClient(*aggregatorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("failed to connect to aggregator at %s: %v", *aggregatorAddr, err)
		}
		client := pb.NewHllServiceClient(grpcConn)

		// Sketch shipping with telemetry.
		go sketchShipLoop(wm, client, *shipInterval, *nodeID, sm, ensemble)

		// Defense polling.
		go defensePollingLoop(client, *nodeID, sm, mc)

		log.Printf("sketch shipping enabled: aggregator=%s interval=%s", *aggregatorAddr, *shipInterval)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	wm.Stop()
	close(attackCh)
	mc.Stop()
	if grpcConn != nil {
		grpcConn.Close()
	}
}

// sketchShipLoop periodically exports the current window sketch and sends it to the aggregator with telemetry.
func sketchShipLoop(wm *window.WindowManager, client pb.HllServiceClient, interval time.Duration, nodeID string, sm *detector.AnomalyStateMachine, ensemble *detector.EnsembleDetector) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		sketch, err := wm.ExportCurrentSketch()
		if err != nil {
			log.Printf("sketch export failed: %v", err)
			continue
		}
		if sketch == nil {
			continue
		}

		req := &pb.MergeRequest{
			Sketch:       sketch,
			NodeId:       nodeID,
			AnomalyState: int32(sm.State()),
		}

		// Add ML telemetry if ensemble is active.
		if ensemble != nil {
			components := ensemble.GetComponents()
			req.LodaScore = components.LODAScore
			req.HstScore = components.HSTScore
			req.EnsembleScore = components.EnsembleScore
		}

		features := wm.LastFeatures()
		req.PacketCount = features.PacketCount

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = client.MergeSketch(ctx, req)
		cancel()
		if err != nil {
			log.Printf("sketch ship failed: %v", err)
		}
	}
}

// defensePollingLoop periodically checks the aggregator for global defense commands.
func defensePollingLoop(client pb.HllServiceClient, nodeID string, sm *detector.AnomalyStateMachine, mc *mitigation.MitigationController) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd, err := client.GetDefenseCommand(ctx, &pb.NodeRequest{NodeId: nodeID})
		cancel()
		if err != nil {
			continue // silently skip on transient errors
		}
		if cmd.Activated {
			if sm.State() != detector.StateUnderAttack {
				log.Printf("[DEFENSE-CMD] global defense activated: reason=%s score=%.3f — forcing UNDER_ATTACK", cmd.Reason, cmd.GlobalScore)
				sm.ForceState(detector.StateUnderAttack)
				mc.UpdateState(detector.StateUnderAttack)
			}
		}
	}
}

// simServer is a minimal gRPC server that accepts InjectIP calls in simulation mode.
type simServer struct {
	pb.UnimplementedHllServiceServer
	ipsChan   chan<- string
	bytesChan chan<- uint64
}

func (s *simServer) InjectIP(_ context.Context, req *pb.InjectIPRequest) (*pb.InjectIPResponse, error) {
	if req.Ip == "" {
		return nil, status.Error(codes.InvalidArgument, "ip is required")
	}
	byteLen := req.ByteLen
	if byteLen == 0 {
		byteLen = 64
	}
	select {
	case s.ipsChan <- req.Ip:
	default:
		// channel full — drop
	}
	select {
	case s.bytesChan <- byteLen:
	default:
	}
	return &pb.InjectIPResponse{}, nil
}

// Forward Health checks in sim mode.
func (s *simServer) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: pb.HealthResponse_SERVING}, nil
}
