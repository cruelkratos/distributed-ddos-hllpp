package detector

// ClassifyAttack uses lightweight heuristic rules to classify attack type.
// No ML models or heavy dependencies — purely rule-based on existing signals.
//
// Heuristic rules:
//   SYN_FLOOD:  high cardinality spike + high rate + low bytes/packet (<100)
//   UDP_FLOOD:  high cardinality spike + high rate + medium bytes/packet (100-500)
//   HTTP_FLOOD: moderate cardinality + high rate + high bytes/packet (>500)
//   SCAN_PROBE: moderate cardinality + low rate + very low bytes/packet
//
// Confidence is the proportion of matching heuristic signals.
func ClassifyAttack(af AttackFeatures) AttackClassification {
	if af.CardinalitySpike < 2.0 && af.RateSpike < 2.0 {
		return AttackClassification{Type: AttackTypeNone, Confidence: 0}
	}

	type candidate struct {
		typ   AttackType
		score float64
		total float64
	}

	syn := candidate{typ: AttackTypeSYNFlood}
	udp := candidate{typ: AttackTypeUDPFlood}
	http := candidate{typ: AttackTypeHTTPFlood}
	scan := candidate{typ: AttackTypeScanProbe}

	// --- SYN_FLOOD rules ---
	syn.total = 3
	if af.CardinalitySpike > 5.0 {
		syn.score++
	}
	if af.RateSpike > 5.0 {
		syn.score++
	}
	if af.BytesPerPacket > 0 && af.BytesPerPacket < 100 {
		syn.score++
	}

	// --- UDP_FLOOD rules ---
	udp.total = 3
	if af.CardinalitySpike > 5.0 {
		udp.score++
	}
	if af.RateSpike > 5.0 {
		udp.score++
	}
	if af.BytesPerPacket >= 100 && af.BytesPerPacket <= 500 {
		udp.score++
	}

	// --- HTTP_FLOOD rules ---
	http.total = 3
	if af.CardinalitySpike > 2.0 && af.CardinalitySpike <= 10.0 {
		http.score++
	}
	if af.RateSpike > 3.0 {
		http.score++
	}
	if af.BytesPerPacket > 500 {
		http.score++
	}

	// --- SCAN_PROBE rules ---
	scan.total = 3
	if af.CardinalitySpike > 2.0 && af.CardinalitySpike <= 10.0 {
		scan.score++
	}
	if af.RateSpike > 0.5 && af.RateSpike <= 3.0 {
		scan.score++
	}
	if af.BytesPerPacket > 0 && af.BytesPerPacket < 80 {
		scan.score++
	}

	candidates := []candidate{syn, udp, http, scan}

	best := candidate{typ: AttackTypeUnknown, score: 0, total: 1}
	for _, c := range candidates {
		conf := c.score / c.total
		bestConf := best.score / best.total
		if conf > bestConf {
			best = c
		}
	}

	confidence := best.score / best.total
	if confidence < 0.33 {
		return AttackClassification{Type: AttackTypeUnknown, Confidence: confidence}
	}
	return AttackClassification{Type: best.typ, Confidence: confidence}
}

// StateTransitionRecord represents a state change event for telemetry.
type StateTransitionRecord struct {
	TimestampUnix int64
	FromState     AnomalyState
	ToState       AnomalyState
	Trigger       string // e.g. "score=0.85", "recovery_count=5"
}

// StateTransitionTracker records recent state transitions.
// Keeps last 10 transitions; memory cost: ~10 * 40 bytes = 400 bytes.
type StateTransitionTracker struct {
	transitions []StateTransitionRecord
}

// Record adds a new transition. The caller provides a Unix timestamp.
func (t *StateTransitionTracker) Record(unixTime int64, from, to AnomalyState, trigger string) {
	rec := StateTransitionRecord{
		TimestampUnix: unixTime,
		FromState:     from,
		ToState:       to,
		Trigger:       trigger,
	}
	t.transitions = append(t.transitions, rec)
	if len(t.transitions) > 10 {
		t.transitions = t.transitions[len(t.transitions)-10:]
	}
}

// Recent returns the most recent N transitions (up to 10).
func (t *StateTransitionTracker) Recent(n int) []StateTransitionRecord {
	if n > len(t.transitions) {
		n = len(t.transitions)
	}
	return t.transitions[len(t.transitions)-n:]
}

// Drain returns all transitions and clears the buffer.
func (t *StateTransitionTracker) Drain() []StateTransitionRecord {
	out := t.transitions
	t.transitions = nil
	return out
}
