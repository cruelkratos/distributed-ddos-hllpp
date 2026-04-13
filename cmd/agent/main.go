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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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
	aggregatorHTTP := flag.String("aggregator-http", "", "HTTP address of aggregator (e.g. http://10.0.0.1:9091). Uses HTTP REST instead of gRPC for sketch shipping.")
	simUDP := flag.Bool("sim-udp", false, "Accept raw UDP IP packets (like ESP32) instead of gRPC InjectIP in sim-mode.")
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

	// Attack classification state.
	temporalBuf := &detector.TemporalBuffer{}
	stateTracker := &detector.StateTransitionTracker{}
	var baselinePackets, baselineBPP float64
	var baselineSamples int
	var lastClassification detector.AttackClassification

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

	// Start resource collector for benchmarking metrics.
	resourceCollector := metrics.NewResourceCollector(5 * time.Second)
	resourceCollector.Start()

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

				prevState := sm.State()
				newState := sm.Transition(score)
				mc.UpdateState(newState)

				// Track state transitions.
				if newState != prevState {
					stateTracker.Record(time.Now().Unix(), prevState, newState,
						fmt.Sprintf("score=%.3f", score))
				}
			} else if attack {
				prevState := sm.State()
				sm.ForceState(detector.StateUnderAttack)
				mc.UpdateState(detector.StateUnderAttack)
				if prevState != detector.StateUnderAttack {
					stateTracker.Record(time.Now().Unix(), prevState, detector.StateUnderAttack, "threshold")
				}
			} else {
				prevState := sm.State()
				newState := sm.Transition(0)
				mc.UpdateState(newState)
				if newState != prevState {
					stateTracker.Record(time.Now().Unix(), prevState, newState, "score=0")
				}
			}

			// Update temporal buffer and run attack classification.
			temporalBuf.Push(float64(cur))
			if sm.State() == detector.StateNormal && baselineSamples < 50 {
				baselinePackets = (baselinePackets*float64(baselineSamples) + float64(features.PacketCount)) / float64(baselineSamples+1)
				if features.PacketCount > 0 {
					bpp := float64(features.ByteVolume) / float64(features.PacketCount)
					baselineBPP = (baselineBPP*float64(baselineSamples) + bpp) / float64(baselineSamples+1)
				}
				baselineSamples++
			}
			if sm.State() != detector.StateNormal {
				af := detector.ExtractAttackFeatures(features, temporalBuf, baselinePackets, baselineBPP)
				af.EnsembleScore = ensembleScore
				lastClassification = detector.ClassifyAttack(af)
			} else {
				lastClassification = detector.AttackClassification{Type: detector.AttackTypeNone}
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

	if *simMode && *simUDP {
		// UDP simulation mode: accept raw newline-delimited IPs via UDP (ESP32-compatible).
		bytesChan = make(chan uint64, *ipsBuf)
		go func() {
			addr, err := net.ResolveUDPAddr("udp", *simGRPCAddr)
			if err != nil {
				log.Fatalf("sim-udp: resolve %s: %v", *simGRPCAddr, err)
			}
			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				log.Fatalf("sim-udp: listen %s: %v", *simGRPCAddr, err)
			}
			log.Printf("sim-udp: listening on %s", *simGRPCAddr)
			buf := make([]byte, 65536)
			for {
				n, _, err := conn.ReadFromUDP(buf)
				if err != nil {
					continue
				}
				// Parse newline-delimited IPs (same format as ESP32 UDP listener).
				start := 0
				for i := 0; i < n; i++ {
					if buf[i] == '\n' {
						ip := string(buf[start:i])
						if ip != "" {
							select {
							case ipsChan <- ip:
							default:
							}
							select {
							case bytesChan <- uint64(i - start):
							default:
							}
						}
						start = i + 1
					}
				}
				// Handle last IP without trailing newline.
				if start < n {
					ip := string(buf[start:n])
					if ip != "" {
						select {
						case ipsChan <- ip:
						default:
						}
						select {
						case bytesChan <- uint64(n - start):
						default:
						}
					}
				}
			}
		}()
	} else if *simMode {
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
	if *aggregatorHTTP != "" {
		// HTTP mode: ship sketches and poll defense via REST (ESP32-compatible).
		go httpSketchShipLoop(wm, *aggregatorHTTP, *shipInterval, *nodeID, sm, ensemble, &lastClassification)
		go httpDefensePollingLoop(*aggregatorHTTP, *nodeID, sm, mc)
		log.Printf("HTTP sketch shipping enabled: aggregator=%s interval=%s", *aggregatorHTTP, *shipInterval)
	} else if *aggregatorAddr != "" {
		var err error
		grpcConn, err = grpc.NewClient(*aggregatorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("failed to connect to aggregator at %s: %v", *aggregatorAddr, err)
		}
		client := pb.NewHllServiceClient(grpcConn)

		// Sketch shipping with telemetry.
		go sketchShipLoop(wm, client, *shipInterval, *nodeID, sm, ensemble, &lastClassification, stateTracker)

		// Defense polling.
		go defensePollingLoop(client, *nodeID, sm, mc)

		log.Printf("gRPC sketch shipping enabled: aggregator=%s interval=%s", *aggregatorAddr, *shipInterval)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	wm.Stop()
	close(attackCh)
	mc.Stop()
	resourceCollector.Stop()
	if grpcConn != nil {
		grpcConn.Close()
	}
}

// sketchShipLoop periodically exports the current window sketch and sends it to the aggregator with telemetry.
func sketchShipLoop(wm *window.WindowManager, client pb.HllServiceClient, interval time.Duration, nodeID string, sm *detector.AnomalyStateMachine, ensemble *detector.EnsembleDetector, classification *detector.AttackClassification, tracker *detector.StateTransitionTracker) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		sketch, err := wm.ExportPreviousSketch()
		if err != nil {
			log.Printf("sketch export failed: %v", err)
			continue
		}
		if sketch == nil {
			log.Printf("[SHIP] sketch is nil, skipping")
			continue
		}
		log.Printf("[SHIP] exporting sketch: p=%d hasData=%v", sketch.P, sketch.Data != nil)

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

		// Add attack classification telemetry.
		if classification != nil {
			req.AttackType = string(classification.Type)
			req.AttackConfidence = classification.Confidence
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = client.MergeSketch(ctx, req)
		cancel()
		if err != nil {
			log.Printf("sketch ship failed: %v", err)
		} else {
			log.Printf("[SHIP] sketch shipped OK to aggregator")
		}
	}
}

// defensePollingLoop periodically checks the aggregator for global defense commands.
// Only escalates the agent from NORMAL → UNDER_ATTACK; does not prevent natural recovery.
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
		if cmd.Activated && sm.State() == detector.StateNormal {
			log.Printf("[DEFENSE-CMD] global defense activated: reason=%s score=%.3f — forcing UNDER_ATTACK", cmd.Reason, cmd.GlobalScore)
			sm.ForceState(detector.StateUnderAttack)
			mc.UpdateState(detector.StateUnderAttack)
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

func (s *simServer) InjectIPs(_ context.Context, req *pb.InjectIPsBatchRequest) (*pb.InjectIPResponse, error) {
	byteLen := req.ByteLen
	if byteLen == 0 {
		byteLen = 64
	}
	for _, ip := range req.Ips {
		select {
		case s.ipsChan <- ip:
		default:
		}
		select {
		case s.bytesChan <- byteLen:
		default:
		}
	}
	return &pb.InjectIPResponse{}, nil
}

// Forward Health checks in sim mode.
func (s *simServer) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: pb.HealthResponse_SERVING}, nil
}

// httpSketchShipLoop periodically exports the current window sketch and ships it via HTTP POST (ESP32-compatible).
func httpSketchShipLoop(wm *window.WindowManager, httpAddr string, interval time.Duration, nodeID string, sm *detector.AnomalyStateMachine, ensemble *detector.EnsembleDetector, classification *detector.AttackClassification) {
	type mergeReq struct {
		NodeID       string  `json:"node_id"`
		P            int     `json:"p"`
		Registers    string  `json:"registers"`
		AnomalyState int32   `json:"anomaly_state"`
		AttackType   string  `json:"attack_type,omitempty"`
		AttackConf   float64 `json:"attack_confidence,omitempty"`
	}

	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		sketch, err := wm.ExportPreviousSketch()
		if err != nil {
			log.Printf("[HTTP-SHIP] sketch export failed: %v", err)
			continue
		}
		if sketch == nil {
			continue
		}

		var regsB64 string
		if dense := sketch.GetDenseData(); dense != nil {
			regsB64 = base64.StdEncoding.EncodeToString(dense)
		} else {
			// No dense data yet — send zero-filled packed registers (6 bits per register).
			m := 1 << uint(sketch.GetP())
			packedSize := (m*6 + 7) / 8
			regsB64 = base64.StdEncoding.EncodeToString(make([]byte, packedSize))
		}
		req := mergeReq{
			NodeID:       nodeID,
			P:            int(sketch.P),
			Registers:    regsB64,
			AnomalyState: int32(sm.State()),
		}
		if classification != nil {
			req.AttackType = string(classification.Type)
			req.AttackConf = classification.Confidence
		}

		body, err := json.Marshal(req)
		if err != nil {
			log.Printf("[HTTP-SHIP] marshal failed: %v", err)
			continue
		}

		resp, err := client.Post(httpAddr+"/api/merge", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("[HTTP-SHIP] POST failed: %v", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		log.Printf("[HTTP-SHIP] sketch shipped OK (HTTP %d)", resp.StatusCode)
	}
}

// httpDefensePollingLoop periodically checks the aggregator for defense commands via HTTP GET.
func httpDefensePollingLoop(httpAddr string, nodeID string, sm *detector.AnomalyStateMachine, mc *mitigation.MitigationController) {
	type defenseResp struct {
		Activated   bool    `json:"activated"`
		GlobalScore float64 `json:"global_score"`
		Reason      string  `json:"reason"`
	}

	client := &http.Client{Timeout: 3 * time.Second}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		resp, err := client.Get(fmt.Sprintf("%s/api/defense?node_id=%s", httpAddr, nodeID))
		if err != nil {
			continue
		}
		var dr defenseResp
		err = json.NewDecoder(resp.Body).Decode(&dr)
		resp.Body.Close()
		if err != nil {
			continue
		}
		if dr.Activated && sm.State() == detector.StateNormal {
			log.Printf("[HTTP-DEFENSE] global defense activated: reason=%s score=%.3f — forcing UNDER_ATTACK", dr.Reason, dr.GlobalScore)
			sm.ForceState(detector.StateUnderAttack)
			mc.UpdateState(detector.StateUnderAttack)
		}
	}
}
