// benchmark-system spawns N synthetic agents, drives traffic through them,
// and records resource metrics (RSS, heap, CPU, goroutines, gRPC overhead)
// to CSV for academic evaluation. Supports scalability experiments.
package main

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/metrics"
	"HLL-BTP/ddos/window"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"time"
)

func main() {
	nodes := flag.Int("nodes", 1, "Number of simulated agent nodes.")
	windowDur := flag.Duration("window", 10*time.Second, "Window duration per node.")
	normalIPs := flag.Int("normal", 300, "Normal distinct IPs per window per node.")
	attackIPs := flag.Int("attack", 15000, "Attack distinct IPs per window per node.")
	totalWindows := flag.Int("windows", 20, "Total windows to simulate.")
	attackFrom := flag.Int("attack-from", 8, "First attack window (0-based).")
	attackCount := flag.Int("attack-count", 6, "Number of consecutive attack windows.")
	sampleInterval := flag.Duration("sample", 2*time.Second, "Resource sampling interval.")
	outCSV := flag.String("out", "benchmark_results.csv", "Output CSV path for resource metrics.")
	detectorType := flag.String("detector", "ensemble", "Detector: threshold, zscore, ensemble")
	flag.Parse()

	attackList := make(map[int]bool)
	for i := 0; i < *attackCount; i++ {
		attackList[*attackFrom+i] = true
	}

	// Build detector.
	var det detector.Detector
	switch *detectorType {
	case "ensemble":
		det = detector.NewEnsembleDetector(42, 0.6, detector.DefaultEnsembleWeights())
	case "zscore":
		det = detector.NewZScoreDetector(20, 3.0)
	default:
		det = detector.NewThresholdDetector(5000)
	}

	// Start resource collector for Prometheus gauges.
	rc := metrics.NewResourceCollector(5 * time.Second)
	rc.Start()
	defer rc.Stop()

	log.Printf("benchmark: nodes=%d windows=%d detector=%s window=%s attack=%v",
		*nodes, *totalWindows, *detectorType, *windowDur, attackList)

	// Create window managers for each simulated node.
	type nodeState struct {
		wm  *window.WindowManager
		rng *rand.Rand
	}
	nodeStates := make([]nodeState, *nodes)
	for i := 0; i < *nodes; i++ {
		wm := window.NewWindowManager(*windowDur, time.Second, det, nil)
		nodeStates[i] = nodeState{
			wm:  wm,
			rng: rand.New(rand.NewSource(int64(42 + i))),
		}
	}
	defer func() {
		for _, ns := range nodeStates {
			ns.wm.Stop()
		}
	}()

	// Open CSV for resource metrics.
	f, err := os.Create(*outCSV)
	if err != nil {
		log.Fatalf("create CSV: %v", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{
		"timestamp", "window_id", "phase", "nodes",
		"rss_bytes", "heap_bytes", "stack_bytes",
		"cpu_goroutines", "total_unique_ips", "detector",
	})

	// Run the simulation window by window.
	for wid := 0; wid < *totalWindows; wid++ {
		phase := "normal"
		if attackList[wid] {
			phase = "attack"
		}

		windowStart := time.Now()

		// Inject traffic into each node.
		for i := 0; i < *nodes; i++ {
			ns := &nodeStates[i]
			count := *normalIPs
			if attackList[wid] {
				count += *attackIPs
			}
			for j := 0; j < count; j++ {
				ip := fmt.Sprintf("%d.%d.%d.%d",
					ns.rng.Intn(256), ns.rng.Intn(256),
					ns.rng.Intn(256), ns.rng.Intn(256))
				ns.wm.Insert(ip)
			}
		}

		// Sample resources periodically during each window.
		sampleTicker := time.NewTicker(*sampleInterval)
		windowDone := time.After(*windowDur)

	sampleLoop:
		for {
			select {
			case <-sampleTicker.C:
				snap := metrics.TakeResourceSnapshot()
				var totalIPs uint64
				for _, ns := range nodeStates {
					totalIPs += ns.wm.CurrentCount()
				}
				w.Write([]string{
					fmt.Sprintf("%d", time.Now().Unix()),
					fmt.Sprintf("%d", wid),
					phase,
					fmt.Sprintf("%d", *nodes),
					fmt.Sprintf("%d", snap.RSSBytes),
					fmt.Sprintf("%d", snap.HeapBytes),
					fmt.Sprintf("%d", snap.StackBytes),
					fmt.Sprintf("%d", runtime.NumGoroutine()),
					fmt.Sprintf("%d", totalIPs),
					*detectorType,
				})
			case <-windowDone:
				sampleTicker.Stop()
				break sampleLoop
			}
		}

		// End-of-window snapshot.
		snap := metrics.TakeResourceSnapshot()
		var totalIPs uint64
		for _, ns := range nodeStates {
			totalIPs += ns.wm.CurrentCount()
		}
		w.Write([]string{
			fmt.Sprintf("%d", time.Now().Unix()),
			fmt.Sprintf("%d", wid),
			phase,
			fmt.Sprintf("%d", *nodes),
			fmt.Sprintf("%d", snap.RSSBytes),
			fmt.Sprintf("%d", snap.HeapBytes),
			fmt.Sprintf("%d", snap.StackBytes),
			fmt.Sprintf("%d", runtime.NumGoroutine()),
			fmt.Sprintf("%d", totalIPs),
			*detectorType,
		})

		elapsed := time.Since(windowStart)
		log.Printf("[window %d/%d] phase=%s nodes=%d unique_ips=%d rss=%dKB heap=%dKB elapsed=%s",
			wid+1, *totalWindows, phase, *nodes, totalIPs,
			snap.RSSBytes/1024, snap.HeapBytes/1024, elapsed)
	}

	log.Printf("benchmark complete: wrote %s", *outCSV)
}
