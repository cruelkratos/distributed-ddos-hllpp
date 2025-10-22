package main

import (
	"HLL-BTP/types/hll" // Main HLL++ package
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync" // Import sync package
	"time"
)

func randomIPv4() string {
	ip := make(net.IP, 4)
	ip[0] = byte(rand.Intn(256))
	ip[1] = byte(rand.Intn(256))
	ip[2] = byte(rand.Intn(256))
	ip[3] = byte(rand.Intn(256))
	return ip.String()
}

func insertWorker(id int, numIPs int, instance *hll.Hllpp_set, uniqueMap map[string]struct{}, mapMutex *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()

	localUnique := make(map[string]struct{}, numIPs)

	for i := 0; i < numIPs; i++ {
		ip := randomIPv4()

		instance.Insert(ip)

		// Add to the local map (no lock needed)
		localUnique[ip] = struct{}{}
	}

	mapMutex.Lock()
	for ip := range localUnique {
		uniqueMap[ip] = struct{}{}
	}
	mapMutex.Unlock()

}

func main() {
	rand.Seed(time.Now().UnixNano())

	// --- Command-Line Flags ---
	algorithmFlag := flag.String("algorithm", "hllpp", "Estimation algorithm ('hll' or 'hllpp')")
	maxIPs := flag.Int("maxIPs", 200000000, "Maximum number of IPs to process.")
	logInterval := flag.Int("logInterval", 1000000, "Log data every N IPs.")
	outputFile := flag.String("outputFile", "benchmarks.txt", "Output file for results.")
	mode := flag.String("mode", "concurrent", "Execution mode: 'concurrent' or 'single'")
	numWorkers := flag.Int("numWorkers", runtime.NumCPU(), "Number of concurrent insertion workers.")

	flag.Parse()

	// --- Setup ---
	isConcurrent := *mode == "concurrent"
	if !isConcurrent && *numWorkers > 1 {
		log.Printf("Warning: Running in single mode but with multiple workers (%d). HLL structure is NOT thread-safe!", *numWorkers)

	}
	if *numWorkers <= 0 {
		*numWorkers = 1 // Ensure at least one worker
	}

	// --- Bias Data Check (Optional based on algorithm) ---
	// if *algorithmFlag == "hllpp" {
	// 	p := general.ConfigPercision()
	// 	dirPath := "bias/biasdata"
	// 	biasOutputFile := filepath.Join(dirPath, fmt.Sprintf("bias_data_p%d.json", p))
	// 	if _, err := os.Stat(biasOutputFile); err != nil && os.IsNotExist(err) {
	// 		fmt.Printf("Bias data file not found at '%s'. Generating now...\n", biasOutputFile)
	// 		bias.GetBiasData(p)
	// 		fmt.Println("Bias data generation complete.")
	// 	} else if err != nil {
	// 		log.Fatalf("Error checking for bias data file: %v", err)
	// 	} else {
	// 		fmt.Printf("Bias data file found at '%s'. Using existing data.\n", biasOutputFile)
	// 	}
	// }

	// --- File Setup ---
	f, err := os.Create(*outputFile)
	if err != nil {
		log.Fatalf("Failed to create output file '%s': %v", *outputFile, err)
	}
	defer f.Close()

	fmt.Printf("Starting streaming benchmark...\n")
	fmt.Printf(" - Algorithm: %s\n", *algorithmFlag)
	fmt.Printf(" - Mode: %s\n", *mode)
	fmt.Printf(" - Workers: %d\n", *numWorkers)
	fmt.Printf(" - Total IPs to process: %d\n", *maxIPs)
	fmt.Printf(" - Recording data every: %d IPs\n", *logInterval)
	fmt.Printf(" - Output will be saved to: %s\n\n", *outputFile)

	// --- Initialization ---
	instance := hll.GetHLLPP(isConcurrent) // Get HLL++ instance
	uniqueIPs := make(map[string]struct{}) // Shared map for true count
	var mapMutex sync.Mutex                // Mutex to protect uniqueIPs map
	var wg sync.WaitGroup                  // WaitGroup to sync workers
	start := time.Now()
	totalProcessed := 0

	// --- Concurrent Benchmark Loop ---
	for totalProcessed < *maxIPs {
		ipsInInterval := *logInterval
		remainingIPs := *maxIPs - totalProcessed
		if ipsInInterval > remainingIPs {
			ipsInInterval = remainingIPs
		}
		if ipsInInterval <= 0 {
			break
		}

		ipsPerWorker := ipsInInterval / *numWorkers
		extraIPs := ipsInInterval % *numWorkers

		wg.Add(*numWorkers)
		for w := 0; w < *numWorkers; w++ {
			workerIPs := ipsPerWorker
			if w < extraIPs {
				workerIPs++
			}
			if workerIPs > 0 {
				go insertWorker(w, workerIPs, instance, uniqueIPs, &mapMutex, &wg)
			} else {
				wg.Done()
			}
		}

		wg.Wait()

		totalProcessed += ipsInInterval

		// --- Log results for this interval ---
		elapsed := time.Since(start)
		estimate := instance.GetElements()

		mapMutex.Lock()
		trueCount := len(uniqueIPs)
		mapMutex.Unlock()

		relError := 0.0
		if trueCount > 0 {
			relError = float64(abs(int64(estimate)-int64(trueCount))) / float64(trueCount) * 100
		}

		outputLine := fmt.Sprintf("Processed %d IPs, Estimate: %d, True: %d, Error: %.5f%%, Time: %.2fs\n",
			totalProcessed, estimate, trueCount, relError, elapsed.Seconds())

		fmt.Print(outputLine)
		_, err := f.WriteString(outputLine)
		if err != nil {
			log.Printf("Warning: failed to write to file: %v", err)
		}
	}

	fmt.Println("\nBenchmark finished successfully!")
}

// abs calculates the absolute value of an int64.
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
