package main

import (
	"HLL-BTP/types/hll"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"time"
)

// randomIPv4 generates a random IPv4 address string for the streaming benchmark.
func randomIPv4() string {
	ip := make(net.IP, 4)
	rand.Read(ip)
	return ip.String()
}

func main() {
	// bias.GetBiasData(14)
	// --- Command-Line Flags ---
	maxIPs := flag.Int("maxIPs", 200000000, "Maximum number of IPs to process for the benchmark.")
	logInterval := flag.Int("logInterval", 1000000, "How often (in number of IPs) to record a data point.")
	outputFile := flag.String("outputFile", "benchmarks.txt", "File to save the benchmark results.")
	algorithm := flag.String("algorithm", "hllpp", "Estimation algorithm to use ('hll' or 'hllpp')")
	mode := flag.String("mode", "concurrent", "Execution mode: 'concurrent' or 'single'")

	numCores := flag.Int("numCores", runtime.NumCPU(), "Number of CPU cores to use")
	flag.Parse()

	// --- Setup ---
	runtime.GOMAXPROCS(*numCores)
	isConcurrent := *mode == "concurrent"

	// --- File Setup ---
	f, err := os.Create(*outputFile)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	fmt.Printf("Starting streaming benchmark...\n")
	fmt.Printf(" - Algorithm: %s\n", *algorithm)
	fmt.Printf(" - Mode: %s\n", *mode)
	fmt.Printf(" - Cores: %d\n", *numCores)
	fmt.Printf(" - Total IPs to process: %d\n", *maxIPs)
	fmt.Printf(" - Recording data every: %d IPs\n", *logInterval)
	fmt.Printf(" - Output will be saved to: %s\n\n", *outputFile)

	// --- Initialization ---
	instance := hll.GetHLL(isConcurrent, *algorithm, false)
	uniqueIPs := make(map[string]struct{})
	start := time.Now()

	// --- Main Benchmark Loop ---
	for i := 1; i <= *maxIPs; i++ {
		// Generate a random IP and insert it into both the HLL and the map.
		ip := randomIPv4()
		instance.Insert(ip)
		uniqueIPs[ip] = struct{}{}

		// At each interval, record the performance metrics.
		if i%*logInterval == 0 {
			elapsed := time.Since(start)
			estimate := instance.GetElements()
			trueCount := len(uniqueIPs)
			relError := 0.0
			if trueCount > 0 {
				relError = float64(abs(int64(estimate)-int64(trueCount))) / float64(trueCount) * 100
			}

			// Format the output string exactly as requested.
			outputLine := fmt.Sprintf("Processed %d IPs, Estimate: %d, True: %d, Error: %.2f%%, Time: %.2fs\n",
				i, estimate, trueCount, relError, elapsed.Seconds())

			// Print to console and write to file.
			fmt.Print(outputLine)
			_, err := f.WriteString(outputLine)
			if err != nil {
				log.Printf("Warning: failed to write to file: %v", err)
			}
		}
	}

	fmt.Println("\nBenchmark finished successfully!")
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
