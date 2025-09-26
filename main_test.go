package main

import (
	"HLL-BTP/types/hll"
	"encoding/csv"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

type BenchmarkResult struct {
	NumInserts   int
	UniqueCount  int
	HLLEstimate  uint64
	MemoryUsedKB float64
	TimeMS       float64
}

func writeResults(results []BenchmarkResult) error {
	f, err := os.Create("benchmark_results.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Write header
	w.Write([]string{"num_inserts", "unique_count", "hll_estimate", "memory_kb", "time_ms"})

	for _, result := range results {
		w.Write([]string{
			fmt.Sprintf("%d", result.NumInserts),
			fmt.Sprintf("%d", result.UniqueCount),
			fmt.Sprintf("%d", result.HLLEstimate),
			fmt.Sprintf("%.2f", result.MemoryUsedKB),
			fmt.Sprintf("%.2f", result.TimeMS),
		})
	}
	return nil
}

func BenchmarkHLLAccuracy(b *testing.B) {
	sizes := []int{1000, 10000, 100000, 1000000}
	results := make([]BenchmarkResult, 0)

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			// Measure baseline memory
			var mBefore runtime.MemStats
			runtime.GC() // Force garbage collection
			runtime.ReadMemStats(&mBefore)

			// Create HLL instance
			instance := hll.GetHLL()
			uniqueIPs := make(map[string]struct{})

			// Measure time
			start := time.Now()

			// Insert IPs
			for i := 0; i < size; i++ {
				ip := randomIPv4()
				instance.Insert(ip)
				uniqueIPs[ip] = struct{}{}
			}

			elapsed := time.Since(start).Milliseconds()

			// Measure final memory
			var mAfter runtime.MemStats
			runtime.GC() // Force garbage collection
			runtime.ReadMemStats(&mAfter)

			// Calculate HLL memory (excluding map)
			hllMemory := float64(mAfter.Alloc-mBefore.Alloc) / 1024

			results = append(results, BenchmarkResult{
				NumInserts:   size,
				UniqueCount:  len(uniqueIPs),
				HLLEstimate:  instance.GetElements(),
				MemoryUsedKB: hllMemory,
				TimeMS:       float64(elapsed),
			})
		})
	}

	if err := writeResults(results); err != nil {
		b.Fatal(err)
	}
}
