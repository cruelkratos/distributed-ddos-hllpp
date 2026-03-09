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
	"net/http"
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

	windowDur time.Duration
	det       detector.Detector

	mu sync.Mutex

	currentID  int64
	currentHLL *hll.Hllpp_set

	previousID  int64
	previousHLL *hll.Hllpp_set
}

func newAggregatorServer(windowDur time.Duration, det detector.Detector) *aggregatorServer {
	return &aggregatorServer{
		windowDur:   windowDur,
		det:         det,
		currentHLL:  hll.GetHLLPP(true),
		previousHLL: hll.GetHLLPP(true),
		currentID:   -1,
		previousID:  -1,
	}
}

func (s *aggregatorServer) nowWindowID(now time.Time) int64 {
	if s.windowDur <= 0 {
		return 0
	}
	return now.UnixNano() / s.windowDur.Nanoseconds()
}

func (s *aggregatorServer) rotateTo(targetID int64) {
	if targetID < 0 {
		return
	}
	if s.currentID == -1 {
		s.currentID = targetID
		s.currentHLL.Reset()
		s.previousID = targetID - 1
		s.previousHLL.Reset()
		return
	}
	for s.currentID < targetID {
		// shift current -> previous, and clear new current
		s.previousID = s.currentID
		s.previousHLL, s.currentHLL = s.currentHLL, s.previousHLL
		s.currentHLL.Reset()
		s.currentID++
	}
}

func (s *aggregatorServer) mergeInto(windowID int64, sketch *pb.Sketch) error {
	if sketch == nil {
		return status.Error(codes.InvalidArgument, "sketch is required")
	}

	temp, err := hll.NewHllppSetFromSketch(sketch)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid sketch: %v", err)
	}

	if windowID == s.currentID {
		return s.currentHLL.MergeSets(temp)
	}
	if windowID == s.previousID {
		return s.previousHLL.MergeSets(temp)
	}
	return status.Errorf(codes.OutOfRange, "window_id=%d not accepted (current=%d previous=%d)", windowID, s.currentID, s.previousID)
}

func (s *aggregatorServer) SubmitWindow(ctx context.Context, req *pb.SubmitWindowRequest) (*pb.SubmitWindowResponse, error) {
	if req == nil || req.GetSketch() == nil {
		return nil, status.Error(codes.InvalidArgument, "request and sketch are required")
	}
	if req.GetWindowId() < 0 {
		return nil, status.Error(codes.InvalidArgument, "window_id must be >= 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure we have current/previous aligned to at least req.window_id.
	s.rotateTo(req.GetWindowId())

	if err := s.mergeInto(req.GetWindowId(), req.GetSketch()); err != nil {
		return nil, err
	}

	return &pb.SubmitWindowResponse{}, nil
}

func (s *aggregatorServer) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: pb.HealthResponse_SERVING}, nil
}

func (s *aggregatorServer) metricsLoop(stop <-chan struct{}, tick time.Duration) {
	if tick <= 0 {
		tick = time.Second
	}
	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-stop:
			return
		case <-t.C:
			now := time.Now()
			s.mu.Lock()
			s.rotateTo(s.nowWindowID(now))
			cur := s.currentHLL.GetElements()
			prev := s.previousHLL.GetElements()
			s.mu.Unlock()

			// Agents submit completed (previous) window sketches, so the peak
			// activity may land in either current or previous depending on
			// timing. Use the larger of the two for attack detection so we
			// catch attacks regardless of which slot holds the merged data.
			activeCount := cur
			if prev > cur {
				activeCount = prev
			}

			attack := false
			if s.det != nil {
				attack = s.det.IsAttack(detector.WindowFeatures{
					CurrentWindowCount:  activeCount,
					PreviousWindowCount: prev,
					WindowDurationSec:   s.windowDur.Seconds(),
				})
			}

			metrics.UpdateWindowMetrics(activeCount, attack, 0)
			if attack {
				log.Printf("[ATTACK][cluster] reason=%s count=%d window_id=%d at=%s",
					s.det.Name(), activeCount, s.nowWindowID(now), now.Format(time.RFC3339))
			}
		}
	}
}

func main() {
	grpcAddr := flag.String("grpc", ":8080", "gRPC listen address.")
	metricsAddr := flag.String("metrics", ":9090", "HTTP address for /metrics.")
	windowDur := flag.Duration("window", 10*time.Second, "Time window for distinct-IP count.")
	threshold := flag.Uint64("threshold", 5000, "Distinct IPs per window above which to signal attack.")
	checkInterval := flag.Duration("check", time.Second, "Interval for detection check and metrics update.")
	flag.Parse()

	det := detector.NewThresholdDetector(*threshold)
	srv := newAggregatorServer(*windowDur, det)

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *grpcAddr, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterHllServiceServer(grpcServer, srv)

	stop := make(chan struct{})

	go srv.metricsLoop(stop, *checkInterval)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())
		if err := http.ListenAndServe(*metricsAddr, mux); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	go func() {
		log.Printf("aggregator started: grpc=%s metrics=%s window=%s threshold=%d", *grpcAddr, *metricsAddr, (*windowDur).String(), *threshold)
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")

	close(stop)
	grpcServer.GracefulStop()
}
