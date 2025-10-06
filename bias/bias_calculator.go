package bias

import (
	"HLL-BTP/models"
	"HLL-BTP/types/hll"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type BiasDataPoint struct {
	RawEstimate float64
	Bias        float64
}

func intToIP(i uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		(i>>24)&0xFF,
		(i>>16)&0xFF,
		(i>>8)&0xFF,
		i&0xFF)
}

func GetBiasData(p int) {
	// instance := hll.GetHLL(false)
	m := 1 << p
	maxCardinality := 5 * m
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	samples := make(map[int]bool)
	for i := 1; i <= 500; i += 10 {
		samples[i] = true
		if rng.Float64() < 0.2 {
			samples[i+1] = true
		}
	}
	for i := 500; i <= 10000; i += 98 {
		samples[i] = true
		if rng.Float64() < 0.24 {
			samples[i+21] = true
		}
	}
	for i := 10000; i <= maxCardinality; i += 1499 {
		samples[i] = true
		if rng.Float64() < 0.3 {
			samples[i+181] = true
		}
	}
	samples[maxCardinality] = true
	var cardinalitySamples []int
	for k := range samples {
		cardinalitySamples = append(cardinalitySamples, k)
	}
	sort.Ints(cardinalitySamples)
	biasData := make([]models.BiasDataPoint, 0, len(cardinalitySamples))
	for _, trueCardinality := range cardinalitySamples {
		var totalEstimate float64 = 0
		for run := 0; run < 200; run++ {
			instance := hll.GetHLL(false, false) // Use non-concurrent non bias corrected
			for i := 1; i <= trueCardinality; i++ {
				ip := intToIP(uint32(i))
				instance.Insert(ip)
			}
			totalEstimate += float64(instance.GetElements())
		}
		avgRawEstimate := totalEstimate / 200.0
		bias := avgRawEstimate - float64(trueCardinality)
		biasData = append(biasData, models.BiasDataPoint{
			RawEstimate: avgRawEstimate,
			Bias:        bias,
		})
	}
	dirPath := "bias/biasdata"
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		panic(fmt.Sprintf("Failed to create directory: %v", err))
	}
	outputFile := filepath.Join(dirPath, fmt.Sprintf("bias_data_p%d.json", p))
	jsonData, err := json.MarshalIndent(biasData, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal JSON: %v", err))
	}

	err = os.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		panic(fmt.Sprintf("Failed to write to file: %v", err))
	}
}
