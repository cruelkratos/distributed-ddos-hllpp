// Aggregator receives HLL++ sketches from distributed agents via gRPC,
// merges them into a global sketch, and runs periodic anomaly detection.
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type aggregatorServer struct {
	pb.UnimplementedHllServiceServer
	globalSet *hll.Hllpp_set
	mu        sync.Mutex
}

func newAggregatorServer() *aggregatorServer {
	return &aggregatorServer{
		globalSet: hll.GetHLLPP(true),
	}
}

// MergeSketch receives a sketch from an agent and merges it into the global set.
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
	return &pb.MergeResponse{}, nil
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

// Insert is not used on the aggregator; agents push sketches, not individual IPs.
func (s *aggregatorServer) Insert(_ context.Context, _ *pb.InsertRequest) (*pb.InsertResponse, error) {
	return nil, status.Error(codes.Unimplemented, "aggregator does not accept individual inserts; use MergeSketch")
}

// detectionLoop runs anomaly detection on the aggregated cardinality at each tick
// and resets the global sketch afterwards (one detection window per tick).
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

	srv := newAggregatorServer()

	// Start detection loop.
	stop := make(chan struct{})
	go srv.detectionLoop(det, *windowDur, stop)

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
