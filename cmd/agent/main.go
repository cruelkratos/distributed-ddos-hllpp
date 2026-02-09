// Agent runs the DDoS detection pipeline: packet capture -> HLL window -> detector -> metrics/alerts.
package main

import (
	"HLL-BTP/ddos/capture"
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/metrics"
	"HLL-BTP/ddos/window"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	iface := flag.String("iface", "", "Network interface for capture (e.g. eth0). Empty for default.")
	windowDur := flag.Duration("window", 10*time.Second, "Time window for distinct-IP count.")
	threshold := flag.Uint64("threshold", 5000, "Distinct IPs per window above which to signal attack.")
	metricsAddr := flag.String("metrics", ":9090", "HTTP address for /metrics.")
	checkInterval := flag.Duration("check", time.Second, "Interval for detection check and metrics update.")
	ipsBuf := flag.Int("ips-buf", 10000, "Buffer size for IP channel.")
	flag.Parse()

	det := detector.NewThresholdDetector(*threshold)
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

	log.Printf("agent started: iface=%q window=%s threshold=%d metrics=%s", *iface, *windowDur, *threshold, *metricsAddr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	wm.Stop()
	close(attackCh)
	ps.Stop()
}
