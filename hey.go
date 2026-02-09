package main

import benchmark "HLL-BTP/benchmarking"

func main1() {
	b := benchmark.NewBenchmarker(8, "hllpp", 12030200, 13100, "benchmarks.txt", "concurrent")
	// we'll run this benchmark with randomly generated ip addr (around 10^7 ips with 8 concurrent workers).

	b.Run()
}
