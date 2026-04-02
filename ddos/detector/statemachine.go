package detector

import "sync"

// AnomalyState represents the current operational state of the node.
type AnomalyState int32

const (
	StateNormal      AnomalyState = 0
	StateUnderAttack AnomalyState = 1
	StateRecovery    AnomalyState = 2
)

// String returns a human-readable state name.
func (s AnomalyState) String() string {
	switch s {
	case StateNormal:
		return "NORMAL"
	case StateUnderAttack:
		return "UNDER_ATTACK"
	case StateRecovery:
		return "RECOVERY"
	default:
		return "UNKNOWN"
	}
}

// AnomalyStateMachine manages transitions between NORMAL, UNDER_ATTACK, and RECOVERY states.
// Transitions are hysteresis-based: requires consecutive confirmations to avoid flapping.
type AnomalyStateMachine struct {
	mu              sync.RWMutex
	state           AnomalyState
	attackThreshold float64 // score above which we consider "attack"
	attackConfirm   int     // consecutive attack windows needed to enter UNDER_ATTACK
	recoveryConfirm int     // consecutive clean windows needed to return to NORMAL from RECOVERY

	consecutiveAttack   int
	consecutiveRecovery int
}

// NewAnomalyStateMachine creates a state machine.
// attackThreshold: score above which to count toward attack confirmation.
// attackConfirm: consecutive windows above threshold before entering UNDER_ATTACK (recommended: 3).
// recoveryConfirm: consecutive clean windows before entering NORMAL from RECOVERY (recommended: 5).
func NewAnomalyStateMachine(attackThreshold float64, attackConfirm, recoveryConfirm int) *AnomalyStateMachine {
	if attackConfirm < 1 {
		attackConfirm = 3
	}
	if recoveryConfirm < 1 {
		recoveryConfirm = 5
	}
	if attackThreshold <= 0 {
		attackThreshold = 0.6
	}
	return &AnomalyStateMachine{
		state:           StateNormal,
		attackThreshold: attackThreshold,
		attackConfirm:   attackConfirm,
		recoveryConfirm: recoveryConfirm,
	}
}

// Transition evaluates a new anomaly score and returns the (possibly changed) state.
func (sm *AnomalyStateMachine) Transition(score float64) AnomalyState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	isAnomaly := score > sm.attackThreshold

	switch sm.state {
	case StateNormal:
		if isAnomaly {
			sm.consecutiveAttack++
			if sm.consecutiveAttack >= sm.attackConfirm {
				sm.state = StateUnderAttack
				sm.consecutiveAttack = 0
				sm.consecutiveRecovery = 0
			}
		} else {
			sm.consecutiveAttack = 0
		}

	case StateUnderAttack:
		if !isAnomaly {
			sm.state = StateRecovery
			sm.consecutiveRecovery = 1
			sm.consecutiveAttack = 0
		}

	case StateRecovery:
		if isAnomaly {
			// Relapse: go back to UNDER_ATTACK immediately.
			sm.state = StateUnderAttack
			sm.consecutiveRecovery = 0
			sm.consecutiveAttack = 0
		} else {
			sm.consecutiveRecovery++
			if sm.consecutiveRecovery >= sm.recoveryConfirm {
				sm.state = StateNormal
				sm.consecutiveRecovery = 0
			}
		}
	}

	return sm.state
}

// State returns the current state.
func (sm *AnomalyStateMachine) State() AnomalyState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

// ForceState sets the state directly (e.g., when aggregator issues defense command).
func (sm *AnomalyStateMachine) ForceState(s AnomalyState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state = s
	sm.consecutiveAttack = 0
	sm.consecutiveRecovery = 0
}
