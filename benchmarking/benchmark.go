package benchmark

import (
	"HLL-BTP/types/hll"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
)

type Benchmarker struct {
	ipList        []string
	numWorkers    int
	algorithmFlag string
	maxIPs        int
	logInterval   int
	outputFile    string
	mode          string
}

func randomIPv4() string {
	// here for backward compatibility
	ip := make(net.IP, 4)
	ip[0] = byte(rand.Intn(256))
	ip[1] = byte(rand.Intn(256))
	ip[2] = byte(rand.Intn(256))
	ip[3] = byte(rand.Intn(256))
	return ip.String()
}

func NewBenchmarker(_numWorkers int, _algorithmFlag string, _maxIPs int, _logInterval int, _outputFile string, _mode string) *Benchmarker {
	start := time.Now()
	generatedIPs := make([]string, _maxIPs)
	for i := 0; i < _maxIPs; i++ {
		generatedIPs[i] = randomIPv4()
	}

	fmt.Printf("Time to Generate IPs: %f", time.Since(start).Seconds())
	return &Benchmarker{
		algorithmFlag: _algorithmFlag,
		logInterval:   _logInterval,
		numWorkers:    _numWorkers,
		maxIPs:        _maxIPs,
		outputFile:    _outputFile,
		mode:          _mode,
		ipList:        generatedIPs,
	}
}

func (b Benchmarker) randomIPv4Generator() string {
	ip := make(net.IP, 4)
	ip[0] = byte(rand.Intn(256))
	ip[1] = byte(rand.Intn(256))
	ip[2] = byte(rand.Intn(256))
	ip[3] = byte(rand.Intn(256))
	return ip.String()
}

func (b *Benchmarker) Run() error {

	isConcurrent := b.mode == "concurrent"
	if !isConcurrent && b.numWorkers > 1 {
		log.Printf("Warning: Running in single mode but with multiple workers (%d). HLL structure is NOT thread-safe!", b.numWorkers)
	}
	if b.numWorkers <= 0 {
		b.numWorkers = 1
	}

	f, err := os.Create(b.outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file '%s': %v", b.outputFile, err)
	}
	defer f.Close()

	fmt.Printf("Starting streaming benchmark...\n")
	fmt.Printf(" - Algorithm: %s\n", b.algorithmFlag)
	fmt.Printf(" - Mode: %s\n", b.mode)
	fmt.Printf(" - Workers: %d\n", b.numWorkers)
	fmt.Printf(" - Total IPs to process: %d\n", b.maxIPs)
	fmt.Printf(" - Recording data every: %d IPs\n", b.logInterval)
	fmt.Printf(" - Output will be saved to: %s\n\n", b.outputFile)

	instance := hll.GetHLLPP(isConcurrent)
	uniqueIPs := make(map[string]struct{})
	var mapMutex sync.Mutex
	var wg sync.WaitGroup
	start := time.Now()
	totalProcessed := 0

	for totalProcessed < b.maxIPs {
		ipsInInterval := b.logInterval
		remainingIPs := b.maxIPs - totalProcessed
		if ipsInInterval > remainingIPs {
			ipsInInterval = remainingIPs
		}
		if ipsInInterval <= 0 {
			break
		}

		ipsPerWorker := ipsInInterval / b.numWorkers
		extraIPs := ipsInInterval % b.numWorkers

		currentIPIndexInList := totalProcessed

		wg.Add(b.numWorkers)
		for w := 0; w < b.numWorkers; w++ {
			workerIPs := ipsPerWorker
			if w < extraIPs {
				workerIPs++
			}
			if workerIPs > 0 {
				startIndex := currentIPIndexInList
				endIndex := currentIPIndexInList + workerIPs
				ipSliceForWorker := b.ipList[startIndex:endIndex]

				go insertWorker(w, ipSliceForWorker, instance, uniqueIPs, &mapMutex, &wg)

				currentIPIndexInList += workerIPs
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
	instance.Reset()
	return nil
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func insertWorker(id int, ipsToInsert []string, instance *hll.Hllpp_set, uniqueMap map[string]struct{}, mapMutex *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()

	// The local map is sized to the number of IPs this worker will process
	localUnique := make(map[string]struct{}, len(ipsToInsert))

	// --- MODIFIED: Iterate over the provided slice ---
	for _, ip := range ipsToInsert {
		instance.Insert(ip)

		// Add to the local map (no lock needed)
		localUnique[ip] = struct{}{}
	}
	// --- End Modification ---

	// Lock once to merge the local map into the global map
	mapMutex.Lock()
	for ip := range localUnique {
		uniqueMap[ip] = struct{}{}
	}
	mapMutex.Unlock()
}

type MemStats struct {
	Alloc     uint64
	HeapAlloc uint64
	HeapInuse uint64
}

func getMemStats() MemStats {
	runtime.GC() // Force GC to get accurate measurement
	runtime.GC() // Call twice to ensure full collection
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return MemStats{
		Alloc:     m.Alloc,
		HeapAlloc: m.HeapAlloc,
		HeapInuse: m.HeapInuse,
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// RunMemoryBenchmark measures memory usage of HLL++ implementation
func (b *Benchmarker) RunMemoryBenchmark() error {
	f, err := os.Create(b.outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file '%s': %v", b.outputFile, err)
	}
	defer f.Close()

	isConcurrent := b.mode == "concurrent"
	p := 14 // Fixed precision
	m := 1 << p

	fmt.Printf("Starting memory benchmark...\n")
	fmt.Printf(" - Algorithm: %s\n", b.algorithmFlag)
	fmt.Printf(" - Mode: %s\n", b.mode)
	fmt.Printf(" - Precision: %d (m=%d registers)\n", p, m)
	fmt.Printf(" - Total IPs to process: %d\n", b.maxIPs)
	fmt.Printf(" - Output will be saved to: %s\n\n", b.outputFile)

	// Calculate theoretical size
	theoreticalBytes := (m * 6) / 8 // 6 bits per register
	fmt.Printf("Theoretical register size: %s\n\n", formatBytes(uint64(theoreticalBytes)))

	// Measure baseline (before creating HLL)
	baseline := getMemStats()
	fmt.Printf("Baseline memory: %s\n", formatBytes(baseline.Alloc))

	// Create HLL instance
	instance := hll.GetHLLPP(isConcurrent)
	afterCreate := getMemStats()
	creationMem := afterCreate.Alloc - baseline.Alloc

	fmt.Printf("After HLL creation: %s (overhead: %s)\n",
		formatBytes(afterCreate.Alloc),
		formatBytes(creationMem))

	headerLine := fmt.Sprintf("%-15s | %-15s | %-15s | %-15s | %-15s | %-10s\n",
		"IPs Processed", "HLL Estimate", "Memory Used", "Delta", "Overhead", "Time(s)")
	fmt.Print(headerLine)
	f.WriteString(headerLine)
	f.WriteString("---------------------------------------------------------------------------------------------------\n")

	totalProcessed := 0
	lastMem := afterCreate.Alloc
	start := time.Now()

	// Process IPs in intervals
	for totalProcessed < b.maxIPs {
		ipsInInterval := b.logInterval
		remainingIPs := b.maxIPs - totalProcessed
		if ipsInInterval > remainingIPs {
			ipsInInterval = remainingIPs
		}
		if ipsInInterval <= 0 {
			break
		}

		// Insert IPs for this interval
		for i := 0; i < ipsInInterval; i++ {
			ip := b.ipList[totalProcessed+i]
			instance.Insert(ip)
		}

		totalProcessed += ipsInInterval

		// Measure memory after insertions
		afterInsert := getMemStats()
		currentMem := afterInsert.Alloc
		totalMem := currentMem - baseline.Alloc
		deltaMem := currentMem - lastMem
		overhead := totalMem - uint64(theoreticalBytes)
		elapsed := time.Since(start)

		estimate := instance.GetElements()

		outputLine := fmt.Sprintf("%-15d | %-15d | %-15s | %-15s | %-15s | %-10.2f\n",
			totalProcessed,
			estimate,
			formatBytes(totalMem),
			formatBytes(deltaMem),
			formatBytes(overhead),
			elapsed.Seconds())

		fmt.Print(outputLine)
		f.WriteString(outputLine)

		lastMem = currentMem
	}

	// Final summary
	finalMem := getMemStats()
	totalMemUsed := finalMem.Alloc - baseline.Alloc
	overhead := totalMemUsed - uint64(theoreticalBytes)
	overheadPercent := float64(overhead) / float64(theoreticalBytes) * 100

	summaryLine := fmt.Sprintf("\n=== Memory Benchmark Summary ===\n"+
		"Theoretical size: %s\n"+
		"Actual memory used: %s\n"+
		"Overhead: %s (%.2f%%)\n"+
		"Total time: %.2fs\n",
		formatBytes(uint64(theoreticalBytes)),
		formatBytes(totalMemUsed),
		formatBytes(overhead),
		overheadPercent,
		time.Since(start).Seconds())

	fmt.Print(summaryLine)
	f.WriteString(summaryLine)

	fmt.Println("\nMemory benchmark finished successfully!")
	instance.Reset()
	return nil
}
