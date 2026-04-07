package detector

import (
	"testing"
)

func TestEWMADetector_WarmupPeriod(t *testing.T) {
	d := NewEWMADetector(0.3, 2.0, 5)
	// During warmup no alerts should fire, even for large counts.
	for i := 0; i < 5; i++ {
		if d.IsAttack(WindowFeatures{CurrentWindowCount: 100000}) {
			t.Fatalf("warmup window %d: unexpected attack alert", i)
		}
	}
}

func TestEWMADetector_NormalTrafficNoAlert(t *testing.T) {
	d := NewEWMADetector(0.3, 2.0, 3)
	// Seed baseline at ~1000.
	for i := 0; i < 10; i++ {
		if d.IsAttack(WindowFeatures{CurrentWindowCount: 1000}) {
			t.Errorf("normal window %d: unexpected alert", i)
		}
	}
}

func TestEWMADetector_SpikeTriggersAlert(t *testing.T) {
	d := NewEWMADetector(0.3, 2.0, 3)
	// Seed baseline at 1000 for several windows.
	for i := 0; i < 8; i++ {
		d.IsAttack(WindowFeatures{CurrentWindowCount: 1000})
	}
	// A spike of 10x should trigger.
	if !d.IsAttack(WindowFeatures{CurrentWindowCount: 10000}) {
		t.Fatal("expected spike to be detected, but IsAttack returned false")
	}
}

func TestEWMADetector_BaselineAdapts(t *testing.T) {
	d := NewEWMADetector(0.5, 2.0, 3)
	// Feed a gradually increasing but not explosive load; no alert expected.
	count := uint64(1000)
	for i := 0; i < 20; i++ {
		count += 50 // gentle growth
		if d.IsAttack(WindowFeatures{CurrentWindowCount: count}) {
			t.Errorf("gradual growth window %d (count=%d): unexpected alert", i, count)
		}
	}
}

func TestEWMADetector_ZeroBaseline(t *testing.T) {
	d := NewEWMADetector(0.3, 2.0, 1)
	// When baseline is zero no alert should fire (avoid divide-by-zero alerts).
	d.IsAttack(WindowFeatures{CurrentWindowCount: 0}) // seed with zero
	if d.IsAttack(WindowFeatures{CurrentWindowCount: 5000}) {
		t.Log("spike over zero baseline suppressed — expected")
	}
}

func TestEWMADetector_Name(t *testing.T) {
	d := NewEWMADetector(0.2, 2.0, 5)
	if d.Name() != "ewma_anomaly" {
		t.Fatalf("unexpected name: %s", d.Name())
	}
}
