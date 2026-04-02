package mitigation

import (
	"HLL-BTP/ddos/detector"
	"sync"
	"time"
)

// MitigationController observes the anomaly state machine and manages the rate limiter.
// When the state is UNDER_ATTACK or RECOVERY, the rate limiter is active.
// When the state returns to NORMAL, the CMS is reset.
type MitigationController struct {
	mu          sync.RWMutex
	limiter     *RateLimiter
	stateMachine *detector.AnomalyStateMachine
	active      bool
	stopDecay   chan struct{}
}

// NewMitigationController creates a controller that links the state machine to the rate limiter.
func NewMitigationController(sm *detector.AnomalyStateMachine, globalRPS float64, perIPLimit uint16) *MitigationController {
	mc := &MitigationController{
		limiter:      NewRateLimiter(globalRPS, perIPLimit),
		stateMachine: sm,
		stopDecay:    make(chan struct{}),
	}
	go mc.decayLoop()
	return mc
}

// decayLoop periodically decays CMS counters to prevent saturation.
func (mc *MitigationController) decayLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-mc.stopDecay:
			return
		case <-ticker.C:
			mc.limiter.DecayCMS()
		}
	}
}

// UpdateState should be called after each anomaly state transition.
// It activates or deactivates the rate limiter based on the current state.
func (mc *MitigationController) UpdateState(state detector.AnomalyState) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	switch state {
	case detector.StateUnderAttack, detector.StateRecovery:
		mc.active = true
	case detector.StateNormal:
		if mc.active {
			mc.limiter.Reset()
		}
		mc.active = false
	}
}

// ShouldAllow returns true if the IP is allowed through.
// When mitigation is not active, always returns true.
func (mc *MitigationController) ShouldAllow(ip string) bool {
	mc.mu.RLock()
	active := mc.active
	mc.mu.RUnlock()

	if !active {
		return true
	}
	return mc.limiter.ShouldAllow(ip)
}

// IsActive returns whether mitigation is currently enabled.
func (mc *MitigationController) IsActive() bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.active
}

// DroppedCount returns cumulative drops from the rate limiter.
func (mc *MitigationController) DroppedCount() uint64 {
	return mc.limiter.DroppedCount()
}

// Stop shuts down the decay goroutine.
func (mc *MitigationController) Stop() {
	close(mc.stopDecay)
}
