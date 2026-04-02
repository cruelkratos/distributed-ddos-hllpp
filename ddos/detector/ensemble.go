package detector

import (
	"math"
	"sync"
)

// EnsembleWeights configures how sub-detector scores are combined.
type EnsembleWeights struct {
	LODA   float64 // default 0.4
	HST    float64 // default 0.3
	ZScore float64 // default 0.2
	EWMA   float64 // default 0.1
}

// DefaultEnsembleWeights returns the default weight configuration.
// Statistical detectors (ZScore, EWMA) are weighted heavily because they
// reliably detect traffic spikes via completed-window counts, whereas
// LODA/HST adapt their internals and may not differentiate well online.
func DefaultEnsembleWeights() EnsembleWeights {
	return EnsembleWeights{LODA: 0.15, HST: 0.15, ZScore: 0.4, EWMA: 0.3}
}

// ScoreComponents holds individual detector scores for telemetry.
type ScoreComponents struct {
	LODAScore     float64
	HSTScore      float64
	ZScoreValue   float64
	EWMAResidual  float64
	EnsembleScore float64
}

// EnsembleDetector combines LODA, HST, ZScore, and EWMA into a weighted ensemble.
// The combined score is in [0, 1] after sigmoid normalization.
type EnsembleDetector struct {
	mu       sync.RWMutex
	loda     *LodaDetector
	hst      *HSTDetector
	zscore   *ZScoreDetector
	ewma     *EWMADetector
	weights  EnsembleWeights
	threshold float64

	// Cached last score components for telemetry.
	lastComponents ScoreComponents
}

// NewEnsembleDetector creates the ensemble with all sub-detectors.
// threshold: anomaly score above which IsAttack returns true (recommended: 0.6).
func NewEnsembleDetector(seed int64, threshold float64, weights EnsembleWeights) *EnsembleDetector {
	if threshold <= 0 || threshold >= 1 {
		threshold = 0.6
	}
	return &EnsembleDetector{
		loda:      NewLodaDetector(seed, 10),
		hst:       NewHSTDetector(seed+1, 50),
		zscore:    NewZScoreDetector(60, 3.0),
		ewma:      NewEWMADetector(0.05, 2.0, 3),
		weights:   weights,
		threshold: threshold,
	}
}

// sigmoid normalizes a raw score to [0,1] range.
// center and scale control the mapping: sigmoid(center) ≈ 0.5.
func sigmoid(x, center, scale float64) float64 {
	return 1.0 / (1.0 + math.Exp(-(x-center)/scale))
}

// Update feeds new features to all sub-detectors.
func (e *EnsembleDetector) Update(f WindowFeatures) {
	fv := ExtractFeatures(f)
	e.loda.Update(fv)
	e.hst.Update(fv)
	// ZScore and EWMA are updated via their IsAttack calls (they update internally).
}

// Score computes the weighted ensemble anomaly score in [0, 1].
// Untrained sub-detectors are excluded and their weight is redistributed.
func (e *EnsembleDetector) Score(f WindowFeatures) float64 {
	fv := ExtractFeatures(f)

	// Get raw scores from ML detectors.
	lodaRaw := e.loda.Score(fv)
	hstRaw := e.hst.Score(fv)

	// Get z-score value from the zscore detector's internal state.
	zScoreNorm := e.computeZScoreNorm(f)

	// Get EWMA residual.
	ewmaResidual := e.computeEWMAResidual(f)

	// Normalize each to [0,1] via sigmoid.
	lodaNorm := sigmoid(lodaRaw, 1.5, 0.8)      // LODA scores: 0.5-2 normal, 2+ anomalous
	hstNorm := sigmoid(hstRaw, 50.0, 20.0)      // HST scores vary widely based on tree depth
	zNorm := sigmoid(zScoreNorm, 3.0, 1.0)      // z-score: 3σ threshold
	ewmaNorm := sigmoid(ewmaResidual, 2.0, 1.0) // EWMA: 2× baseline

	// Adaptive weighting: skip untrained detectors and redistribute weight.
	type detectorContrib struct {
		weight float64
		score  float64
	}
	active := make([]detectorContrib, 0, 4)

	if e.loda.IsTrained() {
		active = append(active, detectorContrib{e.weights.LODA, lodaNorm})
	}
	if e.hst.IsTrained() {
		active = append(active, detectorContrib{e.weights.HST, hstNorm})
	}
	// ZScore and EWMA are always active (statistical, no warmup).
	active = append(active, detectorContrib{e.weights.ZScore, zNorm})
	active = append(active, detectorContrib{e.weights.EWMA, ewmaNorm})

	// Compute total active weight and normalize.
	var totalWeight float64
	for _, a := range active {
		totalWeight += a.weight
	}
	var combined float64
	if totalWeight > 0 {
		for _, a := range active {
			combined += (a.weight / totalWeight) * a.score
		}
	}

	e.mu.Lock()
	e.lastComponents = ScoreComponents{
		LODAScore:     lodaRaw,
		HSTScore:      hstRaw,
		ZScoreValue:   zScoreNorm,
		EWMAResidual:  ewmaResidual,
		EnsembleScore: combined,
	}
	e.mu.Unlock()

	return combined
}

// computeZScoreNorm extracts a z-score-like value using the completed window count.
func (e *EnsembleDetector) computeZScoreNorm(f WindowFeatures) float64 {
	e.zscore.mu.Lock()
	defer e.zscore.mu.Unlock()

	// Use PreviousWindowCount (completed window) for stable baseline comparison.
	current := float64(f.PreviousWindowCount)
	if len(e.zscore.history) < 5 {
		return 0
	}
	mean, stddev := meanStddev(e.zscore.history)
	if stddev == 0 {
		if current > mean {
			return 5.0 // clearly anomalous
		}
		return 0
	}
	return (current - mean) / stddev
}

// computeEWMAResidual returns (current - baseline) / baseline as a deviation ratio.
func (e *EnsembleDetector) computeEWMAResidual(f WindowFeatures) float64 {
	e.ewma.mu.Lock()
	baseline := e.ewma.ewma
	e.ewma.mu.Unlock()

	if baseline <= 0 {
		return 0
	}
	// Use PreviousWindowCount (completed window) for stable comparison.
	return (float64(f.PreviousWindowCount) - baseline) / baseline
}

// GetComponents returns the last computed score components for telemetry.
func (e *EnsembleDetector) GetComponents() ScoreComponents {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastComponents
}

// IsAttack updates all sub-detectors and returns true if ensemble score exceeds threshold.
func (e *EnsembleDetector) IsAttack(f WindowFeatures) bool {
	// Feed PreviousWindowCount to statistical detectors for a stable baseline.
	statF := f
	if f.PreviousWindowCount > 0 {
		statF.CurrentWindowCount = f.PreviousWindowCount
	}

	// Only update statistical baselines when NOT in attack state.
	// This prevents attack traffic from contaminating the reference baseline,
	// which would cause the detector to adapt and lose sensitivity.
	e.mu.RLock()
	lastScore := e.lastComponents.EnsembleScore
	e.mu.RUnlock()

	if lastScore < e.threshold {
		e.zscore.IsAttack(statF)
		e.ewma.IsAttack(statF)
	}

	// Update ML detectors with the original features.
	e.Update(f)

	// Compute ensemble score.
	score := e.Score(f)
	return score > e.threshold
}

// Name satisfies the Detector interface.
func (e *EnsembleDetector) Name() string { return "ensemble" }
