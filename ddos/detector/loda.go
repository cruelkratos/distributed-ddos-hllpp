package detector

import (
	"math"
	"math/rand"
	"sync"
)

const (
	lodaProjections = 40
	lodaBins        = 16
	lodaDims        = 8  // matches FeatureVector length
)

// LodaDetector implements the Lightweight Online Detector of Anomalies (LODA).
// Uses random sparse projections with fixed-width histograms.
// Memory: 40 projections × (16 uint16 bins + 8 float32 weights) ≈ 2.6 KB.
type LodaDetector struct {
	mu          sync.RWMutex
	projections [lodaProjections][lodaDims]float32 // random projection vectors
	histograms  [lodaProjections][lodaBins]uint16  // frequency histograms
	binWidth    [lodaProjections]float32            // width of each bin per projection
	binMin      [lodaProjections]float32            // minimum value seen per projection

	// Welford online normalization per feature dimension.
	count   uint64
	mean    [lodaDims]float64
	m2      [lodaDims]float64
	trained bool // true after warmup period
	warmup  int
}

// IsTrained returns true after the warmup period is complete.
func (ld *LodaDetector) IsTrained() bool {
	ld.mu.RLock()
	defer ld.mu.RUnlock()
	return ld.trained
}

// NewLodaDetector creates a LODA detector with fixed random projections.
// warmup is the number of samples before scoring starts (recommended: 10).
func NewLodaDetector(seed int64, warmup int) *LodaDetector {
	if warmup < 5 {
		warmup = 10
	}
	ld := &LodaDetector{warmup: warmup}
	rng := rand.New(rand.NewSource(seed))

	// Initialize sparse random projections: each projection uses ~half the dimensions.
	for i := 0; i < lodaProjections; i++ {
		for j := 0; j < lodaDims; j++ {
			if rng.Float64() < 0.5 {
				ld.projections[i][j] = float32(rng.NormFloat64())
			}
		}
	}

	// Initialize bin parameters with defaults (will be refined during warmup).
	for i := 0; i < lodaProjections; i++ {
		ld.binWidth[i] = 1.0
		ld.binMin[i] = 0.0
	}
	return ld
}

// normalize applies online Welford normalization to the raw feature vector.
func (ld *LodaDetector) normalize(raw FeatureVector) FeatureVector {
	var norm FeatureVector
	for i := 0; i < lodaDims; i++ {
		variance := 0.0
		if ld.count > 1 {
			variance = ld.m2[i] / float64(ld.count-1)
		}
		std := math.Sqrt(variance)
		if std > 1e-10 {
			norm[i] = (raw[i] - ld.mean[i]) / std
		} else {
			norm[i] = 0
		}
	}
	return norm
}

// project computes the dot product of a projection vector and a feature vector.
func project(proj [lodaDims]float32, fv FeatureVector) float32 {
	var sum float32
	for i := 0; i < lodaDims; i++ {
		sum += proj[i] * float32(fv[i])
	}
	return sum
}

// binIndex returns the histogram bin index for a projected value.
func binIndex(val, binMin, binWidth float32) int {
	if binWidth <= 0 {
		return 0
	}
	idx := int((val - binMin) / binWidth)
	if idx < 0 {
		return 0
	}
	if idx >= lodaBins {
		return lodaBins - 1
	}
	return idx
}

// Update feeds a new sample to the LODA model, updating normalization stats and histograms.
func (ld *LodaDetector) Update(fv FeatureVector) {
	ld.mu.Lock()
	defer ld.mu.Unlock()

	// Welford online mean/variance update.
	ld.count++
	for i := 0; i < lodaDims; i++ {
		delta := fv[i] - ld.mean[i]
		ld.mean[i] += delta / float64(ld.count)
		delta2 := fv[i] - ld.mean[i]
		ld.m2[i] += delta * delta2
	}

	norm := ld.normalize(fv)

	// Update histograms.
	for i := 0; i < lodaProjections; i++ {
		val := project(ld.projections[i], norm)

		// During warmup, expand bin ranges.
		if ld.count <= uint64(ld.warmup) {
			if val < ld.binMin[i] || ld.count == 1 {
				ld.binMin[i] = val
			}
			maxVal := ld.binMin[i] + ld.binWidth[i]*float32(lodaBins)
			if val > maxVal {
				ld.binWidth[i] = (val - ld.binMin[i] + 0.01) / float32(lodaBins)
			}
		}

		idx := binIndex(val, ld.binMin[i], ld.binWidth[i])
		if ld.histograms[i][idx] < math.MaxUint16 {
			ld.histograms[i][idx]++
		}
	}

	if ld.count >= uint64(ld.warmup) {
		ld.trained = true
	}
}

// Score computes the LODA anomaly score: mean negative log-likelihood across projections.
// Higher scores indicate more anomalous inputs.
func (ld *LodaDetector) Score(fv FeatureVector) float64 {
	ld.mu.RLock()
	defer ld.mu.RUnlock()

	if !ld.trained {
		return 0
	}

	norm := ld.normalize(fv)
	var totalScore float64
	var validProjections int

	for i := 0; i < lodaProjections; i++ {
		val := project(ld.projections[i], norm)
		idx := binIndex(val, ld.binMin[i], ld.binWidth[i])

		// Compute density estimate as relative frequency.
		var total uint64
		for j := 0; j < lodaBins; j++ {
			total += uint64(ld.histograms[i][j])
		}
		if total == 0 {
			continue
		}
		freq := float64(ld.histograms[i][idx]) / float64(total)
		if freq < 1e-10 {
			freq = 1e-10
		}
		totalScore += -math.Log(freq)
		validProjections++
	}

	if validProjections == 0 {
		return 0
	}
	return totalScore / float64(validProjections)
}

// Name satisfies the Detector interface.
func (ld *LodaDetector) Name() string { return "loda" }

// IsAttack is not used directly for LODA — use Score() via the ensemble.
// Returns false always; scoring is done by the ensemble.
func (ld *LodaDetector) IsAttack(_ WindowFeatures) bool { return false }
