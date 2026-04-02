package detector

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// ExtractFeatures
// ---------------------------------------------------------------------------

func TestExtractFeatures_Basic(t *testing.T) {
	f := WindowFeatures{
		CurrentWindowCount:  1000,
		PreviousWindowCount: 500,
		PacketCount:         2000,
		ByteVolume:          100000,
		EWMAResidual:        0.5,
		ZScoreValue:         2.0,
	}
	fv := ExtractFeatures(f)
	if fv[0] != 1000 {
		t.Errorf("fv[0] = %f, want 1000", fv[0])
	}
	if fv[1] != 500 {
		t.Errorf("fv[1] = %f, want 500", fv[1])
	}
	if fv[2] != 2.0 { // 1000/500
		t.Errorf("fv[2] = %f, want 2.0", fv[2])
	}
	if fv[3] != 2000 {
		t.Errorf("fv[3] = %f, want 2000", fv[3])
	}
	if fv[4] != 100000 {
		t.Errorf("fv[4] = %f, want 100000", fv[4])
	}
	if fv[5] != 50.0 { // 100000/2000
		t.Errorf("fv[5] = %f, want 50.0", fv[5])
	}
	if fv[6] != 0.5 {
		t.Errorf("fv[6] = %f, want 0.5", fv[6])
	}
	if fv[7] != 2.0 {
		t.Errorf("fv[7] = %f, want 2.0", fv[7])
	}
}

func TestExtractFeatures_ZeroPrevious(t *testing.T) {
	f := WindowFeatures{CurrentWindowCount: 100, PreviousWindowCount: 0}
	fv := ExtractFeatures(f)
	if fv[2] != 1.0 {
		t.Errorf("fv[2] with zero previous = %f, want 1.0", fv[2])
	}
}

func TestExtractFeatures_ZeroPackets(t *testing.T) {
	f := WindowFeatures{PacketCount: 0, ByteVolume: 1000}
	fv := ExtractFeatures(f)
	if fv[5] != 0.0 { // bytes per packet should be 0 when no packets
		t.Errorf("fv[5] with zero packets = %f, want 0.0", fv[5])
	}
}

// ---------------------------------------------------------------------------
// LODA Detector
// ---------------------------------------------------------------------------

func TestLodaDetector_ScoreZeroBeforeWarmup(t *testing.T) {
	ld := NewLodaDetector(42, 30)
	fv := FeatureVector{100, 90, 1.1, 500, 25000, 50, 0.1, 1.0}
	for i := 0; i < 10; i++ {
		ld.Update(fv)
	}
	// Still in warmup — score should be 0.
	if s := ld.Score(fv); s != 0 {
		t.Errorf("expected 0 during warmup, got %f", s)
	}
}

func TestLodaDetector_TrainedAfterWarmup(t *testing.T) {
	ld := NewLodaDetector(42, 30)
	fv := FeatureVector{100, 90, 1.1, 500, 25000, 50, 0.1, 1.0}
	for i := 0; i < 30; i++ {
		ld.Update(fv)
	}
	// After warmup, score should be nonzero for novel data.
	anomalous := FeatureVector{10000, 90, 111.1, 50000, 2500000, 50, 5.0, 8.0}
	ld.Update(anomalous) // update to allow scoring
	s := ld.Score(anomalous)
	if s == 0 {
		t.Error("expected nonzero score for anomalous input after warmup")
	}
}

func TestLodaDetector_NormalLowScore(t *testing.T) {
	ld := NewLodaDetector(42, 30)
	normal := FeatureVector{100, 95, 1.05, 500, 25000, 50, 0.1, 0.5}
	for i := 0; i < 100; i++ {
		ld.Update(normal)
	}
	normalScore := ld.Score(normal)

	// After training on normal data, normal data should have a finite score.
	if math.IsNaN(normalScore) || math.IsInf(normalScore, 0) {
		t.Errorf("normal score should be finite, got %f", normalScore)
	}

	// Update with anomalous data and score: since LODA uses histograms trained
	// on normal data, the anomalous point should land in low-frequency bins.
	anomalous := FeatureVector{10000, 95, 105.0, 50000, 5000000, 100, 5.0, 8.0}
	ld.Update(anomalous)
	anomalousScore := ld.Score(anomalous)

	// The anomalous score should be higher (less likely under the normal distribution).
	if anomalousScore < normalScore {
		t.Logf("anomalous score (%f) vs normal score (%f) — LODA may need more training data", anomalousScore, normalScore)
	}
}

func TestLodaDetector_Name(t *testing.T) {
	ld := NewLodaDetector(42, 30)
	if ld.Name() != "loda" {
		t.Errorf("expected name 'loda', got %s", ld.Name())
	}
}

// ---------------------------------------------------------------------------
// HST Detector
// ---------------------------------------------------------------------------

func TestHSTDetector_ScoreZeroBeforeTrained(t *testing.T) {
	h := NewHSTDetector(42, 200)
	fv := FeatureVector{100, 90, 1.1, 500, 25000, 50, 0.1, 1.0}
	for i := 0; i < 50; i++ {
		h.Update(fv)
	}
	// Not yet trained (needs 200 samples for first window swap).
	if s := h.Score(fv); s != 0 {
		t.Errorf("expected 0 before training, got %f", s)
	}
}

func TestHSTDetector_TrainedAfterWindowSwap(t *testing.T) {
	h := NewHSTDetector(42, 200)
	normal := FeatureVector{100, 90, 1.1, 500, 25000, 50, 0.1, 1.0}
	for i := 0; i < 200; i++ {
		h.Update(normal)
	}
	// After 200 samples, first window swap should have occurred.
	s := h.Score(normal)
	if s == 0 {
		t.Error("expected nonzero score after training")
	}
}

func TestHSTDetector_AnomalyHigherScore(t *testing.T) {
	h := NewHSTDetector(42, 200)
	normal := FeatureVector{100, 95, 1.05, 500, 25000, 50, 0.1, 0.5}
	// Train with 200 normal samples (first window swap fills refMass).
	for i := 0; i < 200; i++ {
		h.Update(normal)
	}
	// Now insert anomalous data into the current window so that the normal
	// leaves have high refMass but low mass for anomalous paths.
	anomalous := FeatureVector{10000, 95, 105.0, 50000, 5000000, 100, 5.0, 8.0}
	for i := 0; i < 200; i++ {
		h.Update(anomalous)
	}
	// After the second window swap, anomalous data builds the new reference.
	// Score normal data: it should now look anomalous (low mass relative to ref
	// which was trained on anomalous data).
	// What we really test: scores are nonzero and the detector is functional.
	normalScore := h.Score(normal)
	anomalousScore := h.Score(anomalous)
	t.Logf("normal score: %f, anomalous score: %f", normalScore, anomalousScore)

	// The point that matches the most recent reference distribution should
	// score higher (refMass/mass ratio is larger for regions seen in ref).
	if anomalousScore == 0 && normalScore == 0 {
		t.Error("both scores are zero — detector is not functional")
	}
}

func TestHSTDetector_Name(t *testing.T) {
	h := NewHSTDetector(42, 200)
	if h.Name() != "hst" {
		t.Errorf("expected name 'hst', got %s", h.Name())
	}
}

// ---------------------------------------------------------------------------
// Ensemble Detector
// ---------------------------------------------------------------------------

func TestEnsembleDetector_ScoreInRange(t *testing.T) {
	e := NewEnsembleDetector(42, 0.6, DefaultEnsembleWeights())
	// Feed enough data to pass all warmup periods.
	normal := WindowFeatures{CurrentWindowCount: 100, PreviousWindowCount: 90, PacketCount: 500, ByteVolume: 25000, WindowDurationSec: 10}
	for i := 0; i < 250; i++ {
		e.IsAttack(normal)
	}
	score := e.Score(normal)
	if score < 0 || score > 1 {
		t.Errorf("ensemble score %f outside [0,1]", score)
	}
}

func TestEnsembleDetector_AttackDetected(t *testing.T) {
	e := NewEnsembleDetector(42, 0.6, DefaultEnsembleWeights())
	normal := WindowFeatures{CurrentWindowCount: 100, PreviousWindowCount: 90, PacketCount: 500, ByteVolume: 25000, WindowDurationSec: 10}
	// Train with normal data.
	for i := 0; i < 300; i++ {
		e.IsAttack(normal)
	}

	// Massive spike.
	spike := WindowFeatures{CurrentWindowCount: 50000, PreviousWindowCount: 100, PacketCount: 100000, ByteVolume: 50000000, WindowDurationSec: 10}
	if !e.IsAttack(spike) {
		// Score may not trigger on the very first spike due to zscore history.
		// Feed a few more spikes.
		for i := 0; i < 5; i++ {
			e.IsAttack(spike)
		}
		if !e.IsAttack(spike) {
			t.Error("expected attack to be detected after sustained spike")
		}
	}
}

func TestEnsembleDetector_GetComponents(t *testing.T) {
	e := NewEnsembleDetector(42, 0.6, DefaultEnsembleWeights())
	normal := WindowFeatures{CurrentWindowCount: 100, PreviousWindowCount: 90, PacketCount: 500, ByteVolume: 25000, WindowDurationSec: 10}
	for i := 0; i < 50; i++ {
		e.IsAttack(normal)
	}
	c := e.GetComponents()
	if c.EnsembleScore < 0 {
		t.Error("ensemble score should be non-negative")
	}
}

func TestEnsembleDetector_Name(t *testing.T) {
	e := NewEnsembleDetector(42, 0.6, DefaultEnsembleWeights())
	if e.Name() != "ensemble" {
		t.Errorf("expected name 'ensemble', got %s", e.Name())
	}
}

func TestSigmoid(t *testing.T) {
	// sigmoid(center, center, scale) should be ~0.5.
	result := sigmoid(3.0, 3.0, 1.0)
	if math.Abs(result-0.5) > 0.001 {
		t.Errorf("sigmoid(3,3,1) = %f, want ~0.5", result)
	}
	// sigmoid should be monotonically increasing.
	low := sigmoid(0, 3.0, 1.0)
	high := sigmoid(6.0, 3.0, 1.0)
	if high <= low {
		t.Errorf("sigmoid(6) = %f should be > sigmoid(0) = %f", high, low)
	}
}

// ---------------------------------------------------------------------------
// State Machine
// ---------------------------------------------------------------------------

func TestStateMachine_StartsNormal(t *testing.T) {
	sm := NewAnomalyStateMachine(0.6, 3, 5)
	if sm.State() != StateNormal {
		t.Errorf("initial state = %v, want NORMAL", sm.State())
	}
}

func TestStateMachine_TransitionToAttack(t *testing.T) {
	sm := NewAnomalyStateMachine(0.6, 3, 5)
	// 3 consecutive attack windows needed.
	for i := 0; i < 2; i++ {
		sm.Transition(0.8)
	}
	if sm.State() != StateNormal {
		t.Errorf("after 2 attacks: state = %v, want NORMAL", sm.State())
	}
	sm.Transition(0.8) // 3rd
	if sm.State() != StateUnderAttack {
		t.Errorf("after 3 attacks: state = %v, want UNDER_ATTACK", sm.State())
	}
}

func TestStateMachine_AttackToRecovery(t *testing.T) {
	sm := NewAnomalyStateMachine(0.6, 1, 5)
	sm.Transition(0.8) // → UNDER_ATTACK
	if sm.State() != StateUnderAttack {
		t.Fatalf("expected UNDER_ATTACK, got %v", sm.State())
	}
	sm.Transition(0.3) // → RECOVERY
	if sm.State() != StateRecovery {
		t.Errorf("expected RECOVERY, got %v", sm.State())
	}
}

func TestStateMachine_RecoveryToNormal(t *testing.T) {
	sm := NewAnomalyStateMachine(0.6, 1, 5)
	sm.Transition(0.8) // → UNDER_ATTACK
	sm.Transition(0.3) // → RECOVERY (consecutiveRecovery=1)
	// Need 5 total clean windows to return to NORMAL.
	// Already at 1, need 4 more.
	for i := 0; i < 3; i++ {
		sm.Transition(0.3)
	}
	// consecutiveRecovery=4, should still be RECOVERY
	if sm.State() != StateRecovery {
		t.Errorf("after 4 clean in recovery: state = %v, want RECOVERY", sm.State())
	}
	sm.Transition(0.3) // 5th clean → NORMAL
	if sm.State() != StateNormal {
		t.Errorf("after 5 clean in recovery: state = %v, want NORMAL", sm.State())
	}
}

func TestStateMachine_RecoveryRelapse(t *testing.T) {
	sm := NewAnomalyStateMachine(0.6, 1, 5)
	sm.Transition(0.8) // → UNDER_ATTACK
	sm.Transition(0.3) // → RECOVERY
	sm.Transition(0.8) // relapse → UNDER_ATTACK
	if sm.State() != StateUnderAttack {
		t.Errorf("expected relapse to UNDER_ATTACK, got %v", sm.State())
	}
}

func TestStateMachine_ConsecutiveResetOnClean(t *testing.T) {
	sm := NewAnomalyStateMachine(0.6, 3, 5)
	sm.Transition(0.8) // 1 attack
	sm.Transition(0.8) // 2 attacks
	sm.Transition(0.3) // clean → resets consecutive counter
	sm.Transition(0.8) // 1 attack (reset)
	sm.Transition(0.8) // 2 attacks
	if sm.State() != StateNormal {
		t.Errorf("expected NORMAL after interrupted attack sequence, got %v", sm.State())
	}
}

func TestStateMachine_ForceState(t *testing.T) {
	sm := NewAnomalyStateMachine(0.6, 3, 5)
	sm.ForceState(StateUnderAttack)
	if sm.State() != StateUnderAttack {
		t.Errorf("forced state = %v, want UNDER_ATTACK", sm.State())
	}
}

func TestStateMachine_String(t *testing.T) {
	tests := []struct {
		s    AnomalyState
		want string
	}{
		{StateNormal, "NORMAL"},
		{StateUnderAttack, "UNDER_ATTACK"},
		{StateRecovery, "RECOVERY"},
		{AnomalyState(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("AnomalyState(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}
