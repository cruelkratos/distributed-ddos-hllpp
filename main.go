package main

import (
	// Main HLL++ package
	benchmark "HLL-BTP/benchmarking"
	"flag"
	"log"
	"runtime"
)

func main() {

	algorithmFlag := flag.String("algorithm", "hllpp", "Estimation algorithm ('hll' or 'hllpp')")
	maxIPs := flag.Int("maxIPs", 200000000, "Maximum number of IPs to process.")
	logInterval := flag.Int("logInterval", 1000000, "Log data every N IPs.")
	outputFile := flag.String("outputFile", "benchmarking/benchmarks.txt", "Output file for results.")
	mode := flag.String("mode", "concurrent", "Execution mode: 'concurrent' or 'single'")
	numWorkers := flag.Int("numWorkers", runtime.NumCPU(), "Number of concurrent insertion workers.")

	flag.Parse()

	b := benchmark.NewBenchmarker(*numWorkers, *algorithmFlag, *maxIPs, *logInterval, *outputFile, *mode)
	if err := b.RunMemoryBenchmark(); err != nil {
		log.Fatalf("Benchmark Failed!!! %v", err)
	}
}
