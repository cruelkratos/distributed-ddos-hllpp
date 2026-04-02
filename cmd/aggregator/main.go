// Aggregator receives HLL++ sketches from distributed agents via gRPC,
// merges them into a global sketch, runs periodic anomaly detection,
// correlates anomaly signals across nodes, and issues defense commands.
package main

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/metrics"
	pb "HLL-BTP/server"
	"HLL-BTP/types/hll"
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Aggregator Prometheus metrics ---

var (
	nodesTotalGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_nodes_total",
		Help: "Total active nodes reporting to aggregator.",
	})
	nodesUnderAttackGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_nodes_under_attack",
		Help: "Number of nodes currently in UNDER_ATTACK state.",
	})
	globalDefenseGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ddos_global_defense_active",
		Help: "1 if global defense is activated, 0 otherwise.",
	})
)

func init() {
	prometheus.MustRegister(nodesTotalGauge, nodesUnderAttackGauge, globalDefenseGauge)
}

// nodeMetrics holds per-node telemetry received from agents.
type nodeMetrics struct {
	ensembleScore float64
	lodaScore     float64
	hstScore      float64
	anomalyState  int32
	packetCount   uint64
	lastSeen      time.Time
}

type aggregatorServer struct {
	pb.UnimplementedHllServiceServer
	globalSet *hll.Hllpp_set
	mu        sync.Mutex

	// Per-node correlation state.
	nodesMu sync.RWMutex
	nodes   map[string]*nodeMetrics

	// Global defense state.
	defenseActivated bool
	defenseScore     float64
	defenseReason    string
	windowDur        time.Duration
}

func newAggregatorServer(windowDur time.Duration) *aggregatorServer {
	return &aggregatorServer{
		globalSet: hll.GetHLLPP(true),
		nodes:     make(map[string]*nodeMetrics),
		windowDur: windowDur,
	}
}

// MergeSketch receives a sketch from an agent, merges it, and records telemetry.
func (s *aggregatorServer) MergeSketch(_ context.Context, req *pb.MergeRequest) (*pb.MergeResponse, error) {
	if req.Sketch == nil {
		return nil, status.Error(codes.InvalidArgument, "sketch is nil")
	}
	incoming, err := hll.NewHllppSetFromSketch(req.Sketch)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "decode sketch: %v", err)
	}
	s.mu.Lock()
	err = s.globalSet.MergeSets(incoming)
	s.mu.Unlock()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "merge: %v", err)
	}

	// Record per-node telemetry.
	if req.NodeId != "" {
		s.nodesMu.Lock()
		s.nodes[req.NodeId] = &nodeMetrics{
			ensembleScore: req.EnsembleScore,
			lodaScore:     req.LodaScore,
			hstScore:      req.HstScore,
			anomalyState:  req.AnomalyState,
			packetCount:   req.PacketCount,
			lastSeen:      time.Now(),
		}
		s.nodesMu.Unlock()
	}

	return &pb.MergeResponse{}, nil
}

// GetDefenseCommand returns the global defense state for a node.
func (s *aggregatorServer) GetDefenseCommand(_ context.Context, _ *pb.NodeRequest) (*pb.DefenseCommand, error) {
	s.nodesMu.RLock()
	defer s.nodesMu.RUnlock()
	return &pb.DefenseCommand{
		Activated:   s.defenseActivated,
		GlobalScore: s.defenseScore,
		Reason:      s.defenseReason,
	}, nil
}

// InjectIP is not supported on the aggregator.
func (s *aggregatorServer) InjectIP(_ context.Context, _ *pb.InjectIPRequest) (*pb.InjectIPResponse, error) {
	return nil, status.Error(codes.Unimplemented, "InjectIP is only supported on agents in simulation mode")
}

// GetEstimate returns the current aggregated cardinality.
func (s *aggregatorServer) GetEstimate(_ context.Context, _ *pb.GetEstimateRequest) (*pb.GetEstimateResponse, error) {
	s.mu.Lock()
	est := s.globalSet.GetElements()
	s.mu.Unlock()
	return &pb.GetEstimateResponse{Estimate: est}, nil
}

// GetSketch exports the global merged sketch.
func (s *aggregatorServer) GetSketch(_ context.Context, _ *pb.GetSketchRequest) (*pb.Sketch, error) {
	s.mu.Lock()
	sketch, err := s.globalSet.ExportSketch()
	s.mu.Unlock()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "export: %v", err)
	}
	return sketch, nil
}

// Reset clears the global sketch.
func (s *aggregatorServer) Reset(_ context.Context, _ *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.mu.Lock()
	s.globalSet.Reset()
	s.mu.Unlock()
	return &pb.ResetResponse{}, nil
}

// Health returns SERVING.
func (s *aggregatorServer) Health(_ context.Context, _ *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: pb.HealthResponse_SERVING}, nil
}

// Insert is not used on the aggregator.
func (s *aggregatorServer) Insert(_ context.Context, _ *pb.InsertRequest) (*pb.InsertResponse, error) {
	return nil, status.Error(codes.Unimplemented, "aggregator does not accept individual inserts; use MergeSketch")
}

// correlationLoop runs cross-node correlation and updates global defense state.
func (s *aggregatorServer) correlationLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(s.windowDur)
	defer ticker.Stop()
	staleTimeout := 2 * s.windowDur
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.runCorrelation(staleTimeout)
		}
	}
}

func (s *aggregatorServer) runCorrelation(staleTimeout time.Duration) {
	now := time.Now()
	s.nodesMu.Lock()

	// Remove stale nodes.
	for id, nm := range s.nodes {
		if now.Sub(nm.lastSeen) > staleTimeout {
			delete(s.nodes, id)
		}
	}

	total := len(s.nodes)
	underAttack := 0
	var maxScore float64
	for _, nm := range s.nodes {
		if nm.anomalyState == int32(detector.StateUnderAttack) {
			underAttack++
		}
		if nm.ensembleScore > maxScore {
			maxScore = nm.ensembleScore
		}
	}

	// Global defense: activate if ≥50% of nodes are under attack.
	activated := false
	reason := ""
	if total > 0 && float64(underAttack)/float64(total) >= 0.5 {
		activated = true
		reason = "majority_nodes_under_attack"
	}

	s.defenseActivated = activated
	s.defenseScore = maxScore
	s.defenseReason = reason
	s.nodesMu.Unlock()

	// Update Prometheus.
	nodesTotalGauge.Set(float64(total))
	nodesUnderAttackGauge.Set(float64(underAttack))

	// Update aggregated per-node score metrics (max across all nodes).
	// Reuses the same gauge names as individual agents so Grafana sees them.
	var maxLoda, maxHST, maxEnsemble float64
	var maxState int32
	for _, nm := range s.nodes {
		if nm.lodaScore > maxLoda {
			maxLoda = nm.lodaScore
		}
		if nm.hstScore > maxHST {
			maxHST = nm.hstScore
		}
		if nm.ensembleScore > maxEnsemble {
			maxEnsemble = nm.ensembleScore
		}
		if nm.anomalyState > maxState {
			maxState = nm.anomalyState
		}
	}
	metrics.LodaScoreGauge.Set(maxLoda)
	metrics.HSTScoreGauge.Set(maxHST)
	metrics.EnsembleScoreGauge.Set(maxEnsemble)
	metrics.AnomalyStateGauge.Set(float64(maxState))

	if activated {
		globalDefenseGauge.Set(1)
	} else {
		globalDefenseGauge.Set(0)
	}

	if activated {
		log.Printf("[GLOBAL-DEFENSE] activated: %d/%d nodes under attack, maxScore=%.3f", underAttack, total, maxScore)
	}
}

// detectionLoop runs anomaly detection on the aggregated cardinality.
func (s *aggregatorServer) detectionLoop(det detector.Detector, windowDur time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(windowDur)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			count := s.globalSet.GetElements()
			s.globalSet.Reset()
			s.mu.Unlock()

			features := detector.WindowFeatures{
				CurrentWindowCount: count,
				WindowDurationSec:  windowDur.Seconds(),
			}
			attack := det.IsAttack(features)

			metrics.UpdateWindowMetrics(count, attack, 0)

			if attack {
				log.Printf("[AGGREGATED-ATTACK] detector=%s count=%d window=%s", det.Name(), count, windowDur)
			} else {
				log.Printf("[AGGREGATOR] count=%d window=%s", count, windowDur)
			}
		}
	}
}

func main() {
	grpcAddr := flag.String("grpc", ":50051", "gRPC listen address.")
	metricsAddr := flag.String("metrics", ":9091", "HTTP address for /metrics.")
	windowDur := flag.Duration("window", 10*time.Second, "Detection window duration.")
	threshold := flag.Uint64("threshold", 5000, "Threshold for threshold detector.")
	detectorType := flag.String("detector", "zscore", "Detector type: threshold, zscore")
	flag.Parse()

	// Allow env overrides for K8s.
	if addr := os.Getenv("GRPC_ADDR"); addr != "" {
		*grpcAddr = addr
	}
	if addr := os.Getenv("METRICS_ADDR"); addr != "" {
		*metricsAddr = addr
	}

	var det detector.Detector
	switch *detectorType {
	case "zscore":
		det = detector.NewZScoreDetector(20, 3.0)
	default:
		det = detector.NewThresholdDetector(*threshold)
	}

	srv := newAggregatorServer(*windowDur)

	// Start detection and correlation loops.
	stop := make(chan struct{})
	go srv.detectionLoop(det, *windowDur, stop)
	go srv.correlationLoop(stop)

	// Start Prometheus metrics.
	go func() {
		if err := metrics.ListenAndServe(*metricsAddr); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	// Start gRPC server.
	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *grpcAddr, err)
	}
	grpcServer := grpc.NewServer()
	pb.RegisterHllServiceServer(grpcServer, srv)

	go func() {
		log.Printf("aggregator gRPC listening on %s, metrics on %s, detector=%s, window=%s", *grpcAddr, *metricsAddr, *detectorType, *windowDur)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC serve: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("aggregator shutting down...")
	close(stop)
	grpcServer.GracefulStop()
}
