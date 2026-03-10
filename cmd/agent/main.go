// Agent runs the DDoS detection pipeline: packet capture -> HLL window -> detector -> metrics/alerts.
package main

import (
	"HLL-BTP/ddos/capture"
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/metrics"
	"HLL-BTP/ddos/window"
	pb "HLL-BTP/server"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	iface := flag.String("iface", "", "Network interface for capture (e.g. eth0). Empty for default.")
	windowDur := flag.Duration("window", 10*time.Second, "Time window for distinct-IP count.")
	threshold := flag.Uint64("threshold", 5000, "Distinct IPs per window above which to signal attack.")
	metricsAddr := flag.String("metrics", ":9090", "HTTP address for /metrics.")
	checkInterval := flag.Duration("check", time.Second, "Interval for detection check and metrics update.")
	ipsBuf := flag.Int("ips-buf", 10000, "Buffer size for IP channel.")
	detectorType := flag.String("detector", "zscore", "Detector type: threshold, zscore")
	aggregatorAddr := flag.String("aggregator", "", "gRPC address of aggregator (e.g. aggregator-service:50051). Empty = standalone.")
	shipInterval := flag.Duration("ship-interval", 0, "Interval for shipping sketches to aggregator. Defaults to window duration.")
	flag.Parse()

	// Allow env override for aggregator address (K8s-friendly).
	if *aggregatorAddr == "" {
		*aggregatorAddr = os.Getenv("AGGREGATOR_ADDR")
	}
	if *shipInterval <= 0 {
		*shipInterval = *windowDur
	}

	var det detector.Detector
	switch *detectorType {
	case "zscore":
		det = detector.NewZScoreDetector(20, 3.0)
	default:
		det = detector.NewThresholdDetector(*threshold)
	}

	attackCh := make(chan window.AttackEvent, 16)
	wm := window.NewWindowManager(*windowDur, *checkInterval, det, attackCh)

	// Goroutine: log attack events
	go func() {
		for ev := range attackCh {
			log.Printf("[ATTACK] reason=%s count=%d window_id=%d at=%s", ev.Reason, ev.Count, ev.WindowID, ev.At.Format(time.RFC3339))
		}
	}()

	// Initial metrics update
	metrics.UpdateWindowMetrics(wm.CurrentCount(), false, wm.ApproxMemoryBytes())

	// Goroutine: periodic metrics update (unique IPs, attack status, memory)
	go func() {
		ticker := time.NewTicker(*checkInterval)
		defer ticker.Stop()
		for range ticker.C {
			cur := wm.CurrentCount()
			prev := wm.PreviousCount()
			attack := det.IsAttack(detector.WindowFeatures{
				CurrentWindowCount:  cur,
				PreviousWindowCount: prev,
				WindowDurationSec:   (*windowDur).Seconds(),
			})
			mem := wm.ApproxMemoryBytes()
			metrics.UpdateWindowMetrics(cur, attack, mem)
		}
	}()

	ipsChan := make(chan string, *ipsBuf)
	ps := capture.NewPcapPacketSource(*iface, 256, false, 100*time.Millisecond)

	// Goroutine: drain IPs into WindowManager
	go func() {
		for ip := range ipsChan {
			_ = wm.Insert(ip)
		}
	}()

	// Goroutine: packet capture
	go func() {
		if err := ps.Run(ipsChan); err != nil {
			log.Printf("capture stopped: %v", err)
		}
		close(ipsChan)
	}()

	// Metrics HTTP server
	go func() {
		if err := metrics.ListenAndServe(*metricsAddr); err != nil {
			log.Printf("metrics server: %v", err)
		}
	}()

	log.Printf("agent started: iface=%q window=%s detector=%s threshold=%d metrics=%s aggregator=%q", *iface, *windowDur, *detectorType, *threshold, *metricsAddr, *aggregatorAddr)

	// gRPC sketch shipping to aggregator (if configured).
	var grpcConn *grpc.ClientConn
	if *aggregatorAddr != "" {
		var err error
		grpcConn, err = grpc.NewClient(*aggregatorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("failed to connect to aggregator at %s: %v", *aggregatorAddr, err)
		}
		client := pb.NewHllServiceClient(grpcConn)
		go sketchShipLoop(wm, client, *shipInterval)
		log.Printf("sketch shipping enabled: aggregator=%s interval=%s", *aggregatorAddr, *shipInterval)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	wm.Stop()
	close(attackCh)
	ps.Stop()
	if grpcConn != nil {
		grpcConn.Close()
	}
}

// sketchShipLoop periodically exports the current window sketch and sends it to the aggregator.
func sketchShipLoop(wm *window.WindowManager, client pb.HllServiceClient, interval time.Duration) {
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = client.MergeSketch(ctx, &pb.MergeRequest{Sketch: sketch})
		cancel()
		if err != nil {
			log.Printf("sketch ship failed: %v", err)
		}
	}
}
