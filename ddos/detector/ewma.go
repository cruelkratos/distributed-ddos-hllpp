package detector

import (
	"math"
	"sync"
)

// EWMADetector uses Exponentially Weighted Moving Average to detect anomalies.
// It maintains a smoothed baseline of traffic counts and flags windows whose
// current count exceeds baseline * (1 + deviationFactor).
//
// The alpha parameter (0 < alpha ≤ 1) controls how quickly the baseline adapts:
//   - alpha close to 1 → fast adaptation (less memory, reacts quickly)
//   - alpha close to 0 → slow adaptation (long memory, less reactive to spikes)
type EWMADetector struct {
	mu              sync.Mutex
	alpha           float64 // smoothing factor
	deviationFactor float64 // proportional threshold above baseline, e.g. 2.0 = 200%
	warmup          int     // number of windows before detection starts
	seen            int
	ewma            float64 // running smoothed estimate
}

// NewEWMADetector creates an EWMA detector.
// alpha: smoothing factor (recommended 0.1–0.3 for DDoS; lower = smoother)
// deviationFactor: multiplier above smoothed baseline to trigger alert (e.g. 2.0)
// warmup: number of windows to seed the baseline before issuing alerts
func NewEWMADetector(alpha, deviationFactor float64, warmup int) *EWMADetector {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.2
	}
	if deviationFactor <= 0 {
		deviationFactor = 2.0
	}
	if warmup < 1 {
		warmup = 5
	}
	return &EWMADetector{
		alpha:           alpha,
		deviationFactor: deviationFactor,
		warmup:          warmup,
	}
}

// IsAttack updates the EWMA and returns true when the current count exceeds
// baseline * (1 + deviationFactor) after the warmup period.
func (e *EWMADetector) IsAttack(f WindowFeatures) bool {
	count := float64(f.CurrentWindowCount)

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.seen == 0 {
		// Seed the EWMA with the first observation.
		e.ewma = count
		e.seen++
		return false
	}

	e.seen++

	// Compute BEFORE updating so we compare against the prior baseline.
	prevEWMA := e.ewma
	e.ewma = e.alpha*count + (1-e.alpha)*e.ewma

	if e.seen <= e.warmup {
		return false
	}

	if prevEWMA == 0 {
		return false
	}

	threshold := prevEWMA * (1 + e.deviationFactor)
	return !math.IsNaN(count) && count > threshold
}

// Name satisfies the Detector interface.
func (e *EWMADetector) Name() string {
	return "ewma_anomaly"
}

// Baseline returns the current smoothed baseline estimate (for diagnostics).
func (e *EWMADetector) Baseline() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ewma
}
