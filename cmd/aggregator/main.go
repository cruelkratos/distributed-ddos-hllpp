// Aggregator receives HLL++ sketches from distributed agents via gRPC,
// merges them into a global sketch, runs periodic anomaly detection,
// correlates anomaly signals across nodes, and issues defense commands.
package main

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/firewall"
	"HLL-BTP/ddos/metrics"
	pb "HLL-BTP/server"
	"HLL-BTP/types/hll"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
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
	attackTypeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ddos_attack_type_classification",
		Help: "Attack type reported by nodes (1=active, label=type).",
	}, []string{"node", "attack_type"})
)

func init() {
	prometheus.MustRegister(nodesTotalGauge, nodesUnderAttackGauge, globalDefenseGauge, attackTypeGauge)
}

// nodeMetrics holds per-node telemetry received from agents.
type nodeMetrics struct {
	ensembleScore    float64
	lodaScore        float64
	hstScore         float64
	anomalyState     int32
	packetCount      uint64
	attackType       string
	attackConfidence float64
	lastSeen         time.Time
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

	// NSG firewall controller (nil if disabled).
	nsg *firewall.NSGController
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
	incomingCount := incoming.GetElements()
	s.mu.Lock()
	beforeCount := s.globalSet.GetElements()
	err = s.globalSet.MergeSets(incoming)
	afterCount := s.globalSet.GetElements()
	s.mu.Unlock()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "merge: %v", err)
	}
	log.Printf("[MERGE] node=%s incoming=%d before=%d after=%d hasSparse=%v hasDense=%v",
		req.NodeId, incomingCount, beforeCount, afterCount,
		req.Sketch.GetSparseData() != nil, req.Sketch.GetDenseData() != nil)

	// Record per-node telemetry.
	if req.NodeId != "" {
		s.nodesMu.Lock()
		s.nodes[req.NodeId] = &nodeMetrics{
			ensembleScore:    req.EnsembleScore,
			lodaScore:        req.LodaScore,
			hstScore:         req.HstScore,
			anomalyState:     req.AnomalyState,
			packetCount:      req.PacketCount,
			attackType:       req.AttackType,
			attackConfidence: req.AttackConfidence,
			lastSeen:         time.Now(),
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

// --- HTTP REST API for lightweight agents (ESP32-C3) ---

type httpMergeRequest struct {
	NodeID        string  `json:"node_id"`
	P             int     `json:"p"`
	Registers     string  `json:"registers"`
	AnomalyState  int32   `json:"anomaly_state"`
	AttackType    string  `json:"attack_type,omitempty"`
	AttackConf    float64 `json:"attack_confidence,omitempty"`
	FreeHeap      uint32  `json:"free_heap,omitempty"`
	ShipLatencyMs float64 `json:"ship_latency_ms,omitempty"`
	LoopTimeUs    uint32  `json:"loop_time_us,omitempty"`
}

func (s *aggregatorServer) handleHTTPMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<18))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var req httpMergeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	denseData, err := base64.StdEncoding.DecodeString(req.Registers)
	if err != nil {
		http.Error(w, "bad base64: "+err.Error(), http.StatusBadRequest)
		return
	}
	sketch := &pb.Sketch{
		P:      int32(req.P),
		PPrime: 25,
		Data:   &pb.Sketch_DenseData{DenseData: denseData},
	}
	incoming, err := hll.NewHllppSetFromSketch(sketch)
	if err != nil {
		http.Error(w, "decode sketch: "+err.Error(), http.StatusBadRequest)
		return
	}
	incomingCount := incoming.GetElements()
	s.mu.Lock()
	beforeCount := s.globalSet.GetElements()
	err = s.globalSet.MergeSets(incoming)
	afterCount := s.globalSet.GetElements()
	s.mu.Unlock()
	if err != nil {
		http.Error(w, "merge: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[MERGE] node=%s incoming=%d before=%d after=%d (HTTP)",
		req.NodeID, incomingCount, beforeCount, afterCount)
	if req.NodeID != "" {
		s.nodesMu.Lock()
		s.nodes[req.NodeID] = &nodeMetrics{
			anomalyState:     req.AnomalyState,
			attackType:       req.AttackType,
			attackConfidence: req.AttackConf,
			lastSeen:         time.Now(),
		}
		s.nodesMu.Unlock()

		// Record ESP32 resource metrics if present.
		if req.FreeHeap > 0 {
			metrics.ESP32FreeHeapGauge.WithLabelValues(req.NodeID).Set(float64(req.FreeHeap))
		}
		if req.ShipLatencyMs > 0 {
			metrics.ESP32ShipLatencyGauge.WithLabelValues(req.NodeID).Set(req.ShipLatencyMs)
		}
		if req.LoopTimeUs > 0 {
			metrics.ESP32LoopTimeGauge.WithLabelValues(req.NodeID).Set(float64(req.LoopTimeUs))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (s *aggregatorServer) handleHTTPDefense(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEFENSE-POLL] from %s", r.RemoteAddr)
	s.nodesMu.RLock()
	resp := struct {
		Activated   bool    `json:"activated"`
		GlobalScore float64 `json:"global_score"`
		Reason      string  `json:"reason"`
	}{
		Activated:   s.defenseActivated,
		GlobalScore: s.defenseScore,
		Reason:      s.defenseReason,
	}
	s.nodesMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

	wasActivated := s.defenseActivated
	s.defenseActivated = activated
	s.defenseScore = maxScore
	s.defenseReason = reason
	s.nodesMu.Unlock()

	// NSG firewall toggle on state transition.
	if s.nsg != nil {
		if activated && !wasActivated {
			log.Printf("[GLOBAL-DEFENSE] NSG lockdown triggered — calling Azure API...")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := s.nsg.Lockdown(ctx); err != nil {
				log.Printf("[GLOBAL-DEFENSE] NSG lockdown FAILED: %v", err)
			} else {
				log.Printf("[GLOBAL-DEFENSE] NSG lockdown SUCCESS")
			}
			metrics.NSGLockdownGauge.Set(1)
			cancel()
		} else if !activated && wasActivated {
			log.Printf("[GLOBAL-DEFENSE] NSG unlock triggered — calling Azure API...")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := s.nsg.Unlock(ctx); err != nil {
				log.Printf("[GLOBAL-DEFENSE] NSG unlock FAILED: %v", err)
			} else {
				log.Printf("[GLOBAL-DEFENSE] NSG unlock SUCCESS")
			}
			metrics.NSGLockdownGauge.Set(0)
			cancel()
		}
	} else if activated && !wasActivated {
		log.Printf("[GLOBAL-DEFENSE] NSG controller is nil — skipping lockdown")
	}

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

	// Update per-node attack type metrics.
	for nodeID, nm := range s.nodes {
		if nm.attackType != "" && nm.attackType != "NONE" {
			attackTypeGauge.WithLabelValues(nodeID, nm.attackType).Set(1)
		} else {
			attackTypeGauge.WithLabelValues(nodeID, "NONE").Set(0)
		}
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
	nsgEnabled := flag.Bool("nsg-enabled", false, "Enable Azure NSG firewall integration.")
	azSubscription := flag.String("azure-subscription-id", "", "Azure subscription ID for NSG.")
	azResourceGroup := flag.String("azure-resource-group", "", "Azure resource group for NSG.")
	azNSGName := flag.String("azure-nsg-name", "", "Azure NSG name.")
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

	// Start resource collector for benchmarking metrics.
	resourceCollector := metrics.NewResourceCollector(5 * time.Second)
	resourceCollector.Start()

	// Initialize NSG firewall controller if enabled.
	if *nsgEnabled {
		subID := *azSubscription
		if v := os.Getenv("AZURE_SUBSCRIPTION_ID"); v != "" {
			subID = v
		}
		rg := *azResourceGroup
		if v := os.Getenv("AZURE_RESOURCE_GROUP"); v != "" {
			rg = v
		}
		nsgName := *azNSGName
		if v := os.Getenv("AZURE_NSG_NAME"); v != "" {
			nsgName = v
		}
		nsgCtrl, err := firewall.NewNSGController(subID, rg, nsgName)
		if err != nil {
			log.Fatalf("failed to create NSG controller: %v", err)
		}
		srv.nsg = nsgCtrl
		log.Printf("NSG firewall integration enabled: subscription=%s rg=%s nsg=%s", subID, rg, nsgName)
	}

	// Start detection and correlation loops.
	stop := make(chan struct{})
	go srv.detectionLoop(det, *windowDur, stop)
	go srv.correlationLoop(stop)

	// Start HTTP server (Prometheus metrics + REST API for ESP32 agents).
	httpMux := http.NewServeMux()
	httpMux.Handle("/metrics", metrics.Handler())
	httpMux.HandleFunc("/api/merge", srv.handleHTTPMerge)
	httpMux.HandleFunc("/api/defense", srv.handleHTTPDefense)
	go func() {
		log.Printf("HTTP server (metrics + REST API) on %s", *metricsAddr)
		if err := http.ListenAndServe(*metricsAddr, httpMux); err != nil {
			log.Printf("HTTP server: %v", err)
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
	resourceCollector.Stop()
	grpcServer.GracefulStop()
}
