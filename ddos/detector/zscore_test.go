package detector

import "testing"

func TestZScoreDetector_NormalTraffic(t *testing.T) {
	det := NewZScoreDetector(20, 3.0)
	// Feed 20 windows of normal traffic (~100 unique IPs each).
	for i := 0; i < 20; i++ {
		f := WindowFeatures{CurrentWindowCount: 100, PreviousWindowCount: 90, WindowDurationSec: 10}
		if det.IsAttack(f) {
			t.Fatalf("false positive at window %d", i)
		}
	}
}

func TestZScoreDetector_SpikeAfterBaseline(t *testing.T) {
	det := NewZScoreDetector(20, 3.0)
	// Build baseline: 20 windows with ~100 unique IPs.
	for i := 0; i < 20; i++ {
		f := WindowFeatures{CurrentWindowCount: 100, PreviousWindowCount: 90, WindowDurationSec: 10}
		det.IsAttack(f) // fill history
	}
	// Inject a spike: 5000 unique IPs.
	spike := WindowFeatures{CurrentWindowCount: 5000, PreviousWindowCount: 100, WindowDurationSec: 10}
	if !det.IsAttack(spike) {
		t.Fatal("expected attack on spike but got false")
	}
}

func TestZScoreDetector_NotEnoughHistory(t *testing.T) {
	det := NewZScoreDetector(20, 3.0)
	// With fewer than 5 data points, should never signal attack.
	for i := 0; i < 4; i++ {
		f := WindowFeatures{CurrentWindowCount: 50000, PreviousWindowCount: 0, WindowDurationSec: 10}
		if det.IsAttack(f) {
			t.Fatalf("should not signal attack with only %d data points", i+1)
		}
	}
}

func TestZScoreDetector_GradualIncrease(t *testing.T) {
	det := NewZScoreDetector(20, 3.0)
	// Gradually increasing traffic should NOT trigger (stays within stddev).
	for i := 0; i < 20; i++ {
		f := WindowFeatures{CurrentWindowCount: uint64(100 + i*5), PreviousWindowCount: 90, WindowDurationSec: 10}
		det.IsAttack(f)
	}
	// Next value slightly above trend should not trigger.
	f := WindowFeatures{CurrentWindowCount: 210, PreviousWindowCount: 190, WindowDurationSec: 10}
	if det.IsAttack(f) {
		t.Fatal("gradual increase should not trigger attack")
	}
}

func TestZScoreDetector_Name(t *testing.T) {
	det := NewZScoreDetector(20, 3.0)
	if det.Name() != "zscore_anomaly" {
		t.Fatalf("expected name zscore_anomaly, got %s", det.Name())
	}
}

func TestMLAnomalyDetector_DelegatesToZScore(t *testing.T) {
	det := NewMLAnomalyDetector()
	// Build baseline.
	for i := 0; i < 20; i++ {
		f := WindowFeatures{CurrentWindowCount: 100, PreviousWindowCount: 90, WindowDurationSec: 10}
		det.IsAttack(f)
	}
	// Spike should be detected (proves it's no longer always-false).
	spike := WindowFeatures{CurrentWindowCount: 5000, PreviousWindowCount: 100, WindowDurationSec: 10}
	if !det.IsAttack(spike) {
		t.Fatal("MLAnomalyDetector should delegate to ZScore and detect spike")
	}
}
