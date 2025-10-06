package hll

import (
	"HLL-BTP/models"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

var biasDataP []byte

type biasCorrector struct {
	data []models.BiasDataPoint
}

var (
	correctorInstance *biasCorrector
	onceCorrector     sync.Once
)

func GetBiasCorrector() *biasCorrector {
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

func (bc *biasCorrector) GetCorrection(rawEstimate float64) float64 {
	const k = 6
	idx := sort.Search(len(bc.data), func(i int) bool {
		return bc.data[i].RawEstimate >= rawEstimate
	})
	start := idx - k/2
	start = max(start, 0)
	end := start + k
	end = min(end, len(bc.data))
	if end == len(bc.data) && end-start != k {
		start = end - k
	}
	if start == 0 && end-start != k {
		end = start + k
	}
	var totalBias float64 = 0
	neighbors := bc.data[start:end]
	for _, neighbor := range neighbors {
		totalBias += neighbor.Bias
	}

	if len(neighbors) == 0 {
		return 0
	}

	return totalBias / float64(len(neighbors))

}
