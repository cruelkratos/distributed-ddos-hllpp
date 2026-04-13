// benchmark-precision compares HLL++ memory usage and accuracy across precisions p=4..14,
// producing a CSV for paper/presentation tables. Demonstrates that even p=4 (12 bytes)
// can detect DDoS-scale traffic spikes despite high standard error.
package main

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/window"
	"HLL-BTP/general"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"time"
)

func main() {
	out := flag.String("out", "precision_benchmark.csv", "Output CSV path.")
	seed := flag.Int64("seed", 1, "RNG seed.")
	flag.Parse()

	precisions := []int{4, 6, 8, 10, 12, 14}
	loads := []int{100, 1_000, 10_000, 100_000}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	_ = w.Write([]string{
		"precision", "m_registers", "sketch_bytes", "theoretical_std_error_pct",
		"distinct_ips", "hll_estimate", "actual_error_pct", "device_fits",
	})

	fmt.Println("Precision | Registers |  Sketch | StdErr%  | Distinct |  Estimate | ActErr% | Fits On")
	fmt.Println("----------|-----------|---------|----------|----------|-----------|---------|--------")

	det := detector.NewThresholdDetector(1 << 30)

	for _, p := range precisions {
		m := 1 << p
		sketchBytes := (m*6 + 7) / 8
		stdErr := 1.04 / math.Sqrt(float64(m)) * 100

		// Determine what device it fits on
		device := "Server"
		if sketchBytes*2 <= 2048-512 { // 2 sketches + 512B overhead for Arduino
			device = "Arduino Uno (2KB)"
		} else if sketchBytes*2 <= 1024*1024 { // Raspberry Pi has 1GB
			device = "Raspberry Pi 3"
		}

		for _, n := range loads {
			// Override precision for this run
			general.SetPrecision(p)

			wm := window.NewWindowManager(time.Hour, time.Hour, det, nil)

			// Reset RNG for reproducibility per (p, n) pair
			rng2 := rand.New(rand.NewSource(*seed + int64(p*1000000+n)))
			seen := make(map[string]struct{}, n)
			for len(seen) < n {
				ip := func() string {
					ipb := make(net.IP, 4)
					for i := range ipb {
						ipb[i] = byte(rng2.Intn(256))
					}
					return ipb.String()
				}()
				seen[ip] = struct{}{}
				_ = wm.Insert(ip)
			}

			estimate := wm.CurrentCount()
			wm.Stop()

			actualErr := math.Abs(float64(estimate)-float64(n)) / float64(n) * 100

			_ = w.Write([]string{
				fmt.Sprintf("%d", p),
				fmt.Sprintf("%d", m),
				fmt.Sprintf("%d", sketchBytes),
				fmt.Sprintf("%.2f", stdErr),
				fmt.Sprintf("%d", n),
				fmt.Sprintf("%d", estimate),
				fmt.Sprintf("%.2f", actualErr),
				device,
			})

			fmt.Printf("    p=%-3d | %7d   | %5dB  | %5.2f%%   | %8d | %9d | %5.1f%%  | %s\n",
				p, m, sketchBytes, stdErr, n, estimate, actualErr, device)
		}
	}

	// Restore default precision
	general.SetPrecision(14)

	log.Printf("Wrote %s", *out)

	// Print summary table for paper
	fmt.Println("\n═══════════════════════════════════════════════════════════════════")
	fmt.Println("Summary: Memory vs Accuracy Trade-off for Paper/Presentation")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Printf("%-10s | %-10s | %-12s | %-15s | %-20s\n", "Precision", "Registers", "Sketch Size", "Std Error", "Target Device")
	fmt.Println("-----------|------------|--------------|-----------------|---------------------")
	for _, p := range precisions {
		m := 1 << p
		bytes := (m*6 + 7) / 8
		stdErr := 1.04 / math.Sqrt(float64(m)) * 100
		device := "Server / Cloud"
		if bytes*2 <= 2048-512 {
			device = "Arduino Uno (2KB)"
		} else if bytes <= 16384 {
			device = "Raspberry Pi 3"
		}
		fmt.Printf("  p=%-6d | %8d   | %8d B   | %6.2f%%          | %s\n", p, m, bytes, stdErr, device)
	}
}
