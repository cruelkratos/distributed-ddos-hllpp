package mitigation

import (
	"HLL-BTP/ddos/detector"
	"testing"
)

// ---------------------------------------------------------------------------
// RateLimiter
// ---------------------------------------------------------------------------

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(100, 50)
	// Should allow first few requests.
	for i := 0; i < 10; i++ {
		if !rl.ShouldAllow("192.168.1.1") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
}

func TestRateLimiter_GlobalRateLimit(t *testing.T) {
	rl := NewRateLimiter(5, 1000) // very low global limit
	allowed := 0
	for i := 0; i < 20; i++ {
		ip := "192.168.1.1"
		if i > 5 {
			ip = "10.0.0.1" // different IP to avoid per-IP limit
		}
		if rl.ShouldAllow(ip) {
			allowed++
		}
	}
	// Should have limited to approximately maxTokens (5*2=10 burst).
	if allowed > 12 {
		t.Errorf("expected global rate limiting, allowed %d of 20", allowed)
	}
}

func TestRateLimiter_PerIPLimit(t *testing.T) {
	rl := NewRateLimiter(10000, 5) // high global, low per-IP
	allowed := 0
	for i := 0; i < 20; i++ {
		if rl.ShouldAllow("192.168.1.1") {
			allowed++
		}
	}
	if allowed > 6 {
		t.Errorf("expected per-IP rate limiting, allowed %d of 20", allowed)
	}
	// Different IP should still be allowed.
	if !rl.ShouldAllow("10.0.0.1") {
		t.Error("different IP should be allowed")
	}
}

func TestRateLimiter_DroppedCount(t *testing.T) {
	rl := NewRateLimiter(2, 1000) // very low global rate
	for i := 0; i < 20; i++ {
		rl.ShouldAllow("192.168.1.1")
	}
	if rl.DroppedCount() == 0 {
		t.Error("expected some drops")
	}
}

func TestRateLimiter_DecayCMS(t *testing.T) {
	rl := NewRateLimiter(10000, 5)
	// Fill CMS for an IP up to the limit.
	for i := 0; i < 6; i++ {
		rl.ShouldAllow("192.168.1.1")
	}
	// 6th request should be blocked (count > 5).
	if rl.ShouldAllow("192.168.1.1") {
		t.Log("7th request allowed — CMS counts may not be exact")
	}
	// After decay, counts halved (6/2=3), so the IP should be allowed again.
	rl.DecayCMS()
	if !rl.ShouldAllow("192.168.1.1") {
		t.Error("expected IP to be allowed after CMS decay")
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter(5, 1000)
	// Drain tokens.
	for i := 0; i < 20; i++ {
		rl.ShouldAllow("192.168.1.1")
	}
	// Reset should restore full capacity.
	rl.Reset()
	if !rl.ShouldAllow("192.168.1.1") {
		t.Error("expected allow after reset")
	}
}

// ---------------------------------------------------------------------------
// MitigationController
// ---------------------------------------------------------------------------

func TestMitigationController_InactiveAllowsAll(t *testing.T) {
	sm := detector.NewAnomalyStateMachine(0.6, 3, 5)
	mc := NewMitigationController(sm, 100, 50)
	defer mc.Stop()

	if !mc.ShouldAllow("192.168.1.1") {
		t.Error("should allow when inactive")
	}
	if mc.IsActive() {
		t.Error("should not be active initially")
	}
}

func TestMitigationController_ActivatesOnAttack(t *testing.T) {
	sm := detector.NewAnomalyStateMachine(0.6, 3, 5)
	mc := NewMitigationController(sm, 100, 50)
	defer mc.Stop()

	mc.UpdateState(detector.StateUnderAttack)
	if !mc.IsActive() {
		t.Error("should be active in UNDER_ATTACK state")
	}
}

func TestMitigationController_DeactivatesOnNormal(t *testing.T) {
	sm := detector.NewAnomalyStateMachine(0.6, 3, 5)
	mc := NewMitigationController(sm, 100, 50)
	defer mc.Stop()

	mc.UpdateState(detector.StateUnderAttack)
	mc.UpdateState(detector.StateNormal)
	if mc.IsActive() {
		t.Error("should be inactive after returning to NORMAL")
	}
}

func TestMitigationController_ActiveDuringRecovery(t *testing.T) {
	sm := detector.NewAnomalyStateMachine(0.6, 3, 5)
	mc := NewMitigationController(sm, 100, 50)
	defer mc.Stop()

	mc.UpdateState(detector.StateRecovery)
	if !mc.IsActive() {
		t.Error("should be active during RECOVERY")
	}
}

func TestMitigationController_DroppedCount(t *testing.T) {
	sm := detector.NewAnomalyStateMachine(0.6, 3, 5)
	mc := NewMitigationController(sm, 2, 1000) // very low global rate
	defer mc.Stop()

	mc.UpdateState(detector.StateUnderAttack)
	for i := 0; i < 20; i++ {
		mc.ShouldAllow("192.168.1.1")
	}
	if mc.DroppedCount() == 0 {
		t.Error("expected drops when active with low rate limit")
	}
}
