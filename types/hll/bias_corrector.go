package hll

import (
	"HLL-BTP/models"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

//go:embed bias/biasdata/bias_data_p14.json
var biasDataP []byte

type biasCorrector struct {
	data []models.BiasDataPoint
}

var (
	correctorInstance *biasCorrector
	onceCorrector     sync.Once
)

func getbiascorrector() *biasCorrector {
	onceCorrector.Do(func() {
		var data []models.BiasDataPoint
		if err := json.Unmarshal(biasDataP, &data); err != nil {
			panic(fmt.Sprintf("Failed to parse embedded bias data: %v", err))
		}

		// Sort the data by RawEstimate to enable binary search.
		sort.Slice(data, func(i, j int) bool {
			return data[i].RawEstimate < data[j].RawEstimate
		})

		correctorInstance = &biasCorrector{data: data}
	})
	return correctorInstance
}

// getCorrection uses k-NN interpolation to find the appropriate bias correction.
func (c *biasCorrector) getCorrection(rawEstimate float64) float64 {
	if len(c.data) == 0 {
		return 0 // No data, no correction.
	}

	const k = 6 // Number of nearest neighbors to consider.

	// Use binary search to find the insertion point for the rawEstimate.
	idx := sort.Search(len(c.data), func(i int) bool {
		return c.data[i].RawEstimate >= rawEstimate
	})

	// --- CORRECTED WINDOWING LOGIC ---
	// Define the ideal window start and end.
	start := idx - k/2
	end := idx + k/2

	// Adjust the window if it goes out of bounds.
	if start < 0 {
		// If the window goes past the beginning, shift it right.
		start = 0
		end = k
	}
	if end > len(c.data) {
		// If the window goes past the end, shift it left.
		end = len(c.data)
		start = end - k
	}

	// Final safety check in case the dataset has fewer than k points.
	if start < 0 {
		start = 0
	}
	// --- END OF CORRECTION ---

	neighbors := c.data[start:end]
	if len(neighbors) == 0 {
		return 0
	}

	// Average the bias of the k-nearest neighbors.
	var totalBias float64
	for _, point := range neighbors {
		totalBias += point.Bias
	}
	return totalBias / float64(len(neighbors))
}
