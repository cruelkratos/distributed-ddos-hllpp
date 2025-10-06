package main

import (
	"HLL-BTP/bias"
	"HLL-BTP/general"
	"crypto/rand"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// abs calculates the absolute value of an int64.
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// randomIPv4 generates a random IPv4 address string.
func randomIPv4() string {
	ip := make(net.IP, 4)
	rand.Read(ip)
	return ip.String()
}

func main() {
	// --- Command-Line Flags for Configuration ---
	// HLL algorithm setting

	dirPath := "bias/biasdata"
	p := general.ConfigPercision()
	_outputFile := filepath.Join(dirPath, fmt.Sprintf("bias_data_p%d.json", p))
	if _, err := os.Stat(_outputFile); err == nil {
		fmt.Printf("Bias data file already exists at '%s'.\n", _outputFile)
		fmt.Println("Skipping generation. Delete the file if you want to regenerate it.")

	} else if !os.IsNotExist(err) {
		panic(fmt.Sprintf("Error checking for bias data file: %v", err))
	} else {
		bias.GetBiasData(p)
	}

	// mode := flag.String("mode", "concurrent", "Execution mode: 'concurrent' or 'single'")
	algorithm := flag.String("algorithm", "hllpp", "Estimation algorithm to use ('hll' or 'hllpp')")
	fmt.Printf(*algorithm)
	// // Benchmark settings
	// maxIPs := flag.Int("maxIPs", 200000000, "Maximum number of IPs to process for the benchmark.")
	// stepSize := flag.Int("stepSize", 1000000, "How often (in number of IPs) to record a data point.")
	// outputFile := flag.String("outputFile", "benchmarks.txt", "File to save the benchmark results.")
	// flag.Parse()

	// // --- File Setup ---
	// // Open the output file for writing. Create it if it doesn't exist, or truncate it if it does.
	// f, err := os.Create(*outputFile)
	// if err != nil {
	// 	log.Fatalf("Failed to create output file: %v", err)
	// }
	// defer f.Close()

	// fmt.Printf("Starting benchmark...\n")
	// fmt.Printf(" - Mode: %s\n", *mode)
	// fmt.Printf(" - Total IPs to process: %d\n", *maxIPs)
	// fmt.Printf(" - Recording data every: %d IPs\n", *stepSize)
	// fmt.Printf(" - Output will be saved to: %s\n\n", *outputFile)

	// // --- Initialization ---
	// // Initialize HLL based on the selected flags.
	// instance := hll.GetHLL(*mode == "concurrent")
	// // Use a map to keep track of the true unique count.
	// uniqueIPs := make(map[string]struct{})
	// // Record the start time of the benchmark.
	// start := time.Now()

	// // --- Benchmark Loop ---
	// for i := 0; i <= *maxIPs; i++ {
	// 	// Generate a random IP and insert it into both the HLL and the map.
	// 	ip := randomIPv4()
	// 	instance.Insert(ip)
	// 	uniqueIPs[ip] = struct{}{}

	// 	// At each step, record the performance metrics.
	// 	if i%*stepSize == 0 {
	// 		elapsed := time.Since(start)
	// 		estimate := instance.GetElements()
	// 		trueCount := len(uniqueIPs)
	// 		relError := 0.0
	// 		if trueCount > 0 {
	// 			relError = float64(abs(int64(estimate)-int64(trueCount))) / float64(trueCount) * 100
	// 		}

	// 		// Format the output string exactly as requested.
	// 		outputLine := fmt.Sprintf("Processed %d IPs, Estimate: %d, True: %d, Error: %.2f%%, Time: %.2fs\n",
	// 			i, estimate, trueCount, relError, elapsed.Seconds())

	// 		// Print to console and write to file.
	// 		fmt.Print(outputLine)
	// 		_, err := f.WriteString(outputLine)
	// 		if err != nil {
	// 			log.Printf("Warning: failed to write to file: %v", err)
	// 		}
	// 	}
	// }

	// fmt.Println("\nBenchmark finished successfully!")
}
