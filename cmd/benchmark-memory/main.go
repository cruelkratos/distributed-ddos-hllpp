// benchmark-memory compares HLL++ (WindowManager) memory vs exact counting (map) for report.
package main

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/window"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"time"
)

func main() {
	out := flag.String("out", "memory_benchmark.csv", "Output CSV path.")
	windowDur := flag.Duration("window", 10*time.Second, "Window duration (for WindowManager).")
	seed := flag.Int64("seed", 1, "RNG seed.")
	flag.Parse()

	// Distinct IP counts to test (plan: 1k, 10k, 100k, 1M)
	loads := []int{1_000, 10_000, 100_000, 1_000_000}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	_ = w.Write([]string{"distinct_ips", "hll_memory_bytes", "exact_memory_bytes", "hll_estimate"})

	rng := rand.New(rand.NewSource(*seed))
	genIP := func() string {
		ip := make(net.IP, 4)
		for i := range ip {
			ip[i] = byte(rng.Intn(256))
		}
		return ip.String()
	}

	det := detector.NewThresholdDetector(1 << 30) // no attack

	for _, n := range loads {
		wm := window.NewWindowManager(*windowDur, time.Second, det, nil)
		seen := make(map[string]struct{}, n)
		for len(seen) < n {
			ip := genIP()
			seen[ip] = struct{}{}
			_ = wm.Insert(ip)
		}
		estimate := wm.CurrentCount()
		hllMem := wm.ApproxMemoryBytes()
		wm.Stop()

		// Exact counting: measure heap delta for map with n distinct IPs
		runtime.GC()
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		exactMap := make(map[string]struct{}, n)
		rng2 := rand.New(rand.NewSource(*seed + int64(n)))
		for len(exactMap) < n {
			ip := make(net.IP, 4)
			for j := range ip {
				ip[j] = byte(rng2.Intn(256))
			}
			exactMap[ip.String()] = struct{}{}
		}
		runtime.GC()
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		exactMem := after.HeapAlloc - before.HeapAlloc
		_ = exactMap

		_ = w.Write([]string{
			fmt.Sprintf("%d", n),
			fmt.Sprintf("%d", hllMem),
			fmt.Sprintf("%d", exactMem),
			fmt.Sprintf("%d", estimate),
		})
		fmt.Printf("distinct_ips=%d hll_memory_bytes=%d exact_memory_bytes=%d hll_estimate=%d\n",
			n, hllMem, exactMem, estimate)
	}

	log.Printf("Wrote %s", *out)
}
