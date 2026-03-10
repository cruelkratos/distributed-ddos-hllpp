package detector

import (
	"math"
	"sync"
)

// ZScoreDetector is a statistical anomaly detector that uses Z-score analysis
// over a sliding history of window cardinalities. It replaces the previous
// MLAnomalyDetector stub with a lightweight, adaptive baseline approach.
type ZScoreDetector struct {
	history     []float64
	maxHistory  int
	sensitivity float64
	mu          sync.Mutex
}

// NewZScoreDetector creates a Z-score detector.
//   - maxHistory: number of past windows to keep for baseline (recommended: 20).
//   - sensitivity: number of standard deviations above mean to trigger (recommended: 3.0).
func NewZScoreDetector(maxHistory int, sensitivity float64) *ZScoreDetector {
	if maxHistory < 5 {
		maxHistory = 5
	}
	return &ZScoreDetector{
		history:     make([]float64, 0, maxHistory),
		maxHistory:  maxHistory,
		sensitivity: sensitivity,
	}
}

// IsAttack returns true when the current window count is more than
// sensitivity standard deviations above the historical mean.
// Requires at least 5 historical data points before it can signal.
func (z *ZScoreDetector) IsAttack(f WindowFeatures) bool {
	z.mu.Lock()
	defer z.mu.Unlock()

	current := float64(f.CurrentWindowCount)

	// Need prior history (excluding current) to compute a baseline.
	if len(z.history) >= 5 {
		mean, stddev := meanStddev(z.history)
		// Avoid division by zero: if stddev is 0, any value above mean is anomalous.
		if stddev == 0 {
			if current > mean {
				z.appendHistory(current)
				return true
			}
			z.appendHistory(current)
			return false
		}
		zScore := (current - mean) / stddev
		z.appendHistory(current)
		return zScore > z.sensitivity
	}

	z.appendHistory(current)
	return false
}

func (z *ZScoreDetector) appendHistory(v float64) {
	if len(z.history) >= z.maxHistory {
		// FIFO: drop oldest
		copy(z.history, z.history[1:])
		z.history[len(z.history)-1] = v
	} else {
		z.history = append(z.history, v)
	}
}

// Name returns the detector name for metrics and AttackEvent.Reason.
func (z *ZScoreDetector) Name() string {
	return "zscore_anomaly"
}

func meanStddev(data []float64) (float64, float64) {
	n := float64(len(data))
	var sum float64
	for _, v := range data {
		sum += v
	}
	mean := sum / n
	var sqDiffSum float64
	for _, v := range data {
		d := v - mean
		sqDiffSum += d * d
	}
	stddev := math.Sqrt(sqDiffSum / n)
	return mean, stddev
}
