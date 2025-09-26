package main

import (
	"HLL-BTP/types/hll"
	"crypto/rand"
	"fmt"
	"net"
	"runtime"
	"time"
)

func randomIPv4() string {
	ip := make(net.IP, 4)
	rand.Read(ip)
	return ip.String()
}

func main() {
	// Memory baseline
	var mBefore runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&mBefore)

	instance := hll.GetHLL()
	uniqueIPs := make(map[string]struct{}, 1000000) // Pre-allocate map
	total := 191326545
	batchSize := 100000

	// Time measurement
	start := time.Now()

	// Process in batches
	for i := 0; i < total; i += batchSize {
		for j := 0; j < batchSize && (i+j) < total; j++ {
			ip := randomIPv4()
			instance.Insert(ip)
			uniqueIPs[ip] = struct{}{}
		}

		if i%1000000 == 0 {
			elapsed := time.Since(start)
			estimate := instance.GetElements()
			trueCount := len(uniqueIPs)
			relError := float64(abs(int64(estimate)-int64(trueCount))) / float64(trueCount) * 100
			fmt.Printf("Processed %d IPs, Estimate: %d, True: %d, Error: %.2f%%, Time: %.2fs\n",
				i, estimate, trueCount, relError, elapsed.Seconds())
		}
	}

	// Final measurements
	elapsed := time.Since(start)
	estimate := instance.GetElements()
	trueCount := len(uniqueIPs)
	relError := float64(abs(int64(estimate)-int64(trueCount))) / float64(trueCount) * 100

	var mAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&mAfter)

	fmt.Printf("\nFinal Results:\n")
	fmt.Printf("Total insertions: %d\n", total)
	fmt.Printf("True unique count: %d\n", trueCount)
	fmt.Printf("HLL Estimate: %d\n", estimate)
	fmt.Printf("Relative Error: %.2f%%\n", relError)
	fmt.Printf("Time taken: %.2f seconds\n", elapsed.Seconds())
	fmt.Printf("Memory used: %.2f KB\n", float64(mAfter.Alloc-mBefore.Alloc)/1024)
	fmt.Printf("Average insertion rate: %.2f ops/sec\n",
		float64(total)/elapsed.Seconds())
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
