package detector

// AttackType represents a classified DDoS attack category.
type AttackType string

const (
	AttackTypeNone      AttackType = "NONE"
	AttackTypeSYNFlood  AttackType = "SYN_FLOOD"
	AttackTypeUDPFlood  AttackType = "UDP_FLOOD"
	AttackTypeHTTPFlood AttackType = "HTTP_FLOOD"
	AttackTypeScanProbe AttackType = "SCAN_PROBE"
	AttackTypeUnknown   AttackType = "UNKNOWN"
)

// AttackClassification holds the inferred attack type and confidence.
type AttackClassification struct {
	Type       AttackType
	Confidence float64 // 0.0 to 1.0
}

// AttackFeatures holds derived signals for classification.
type AttackFeatures struct {
	CardinalitySpike  float64 // current / previous unique IPs
	RateSpike         float64 // current packets / baseline packets
	BytesPerPacket    float64 // avg bytes per packet this window
	BaselineBPP       float64 // baseline bytes per packet
	TemporalVariance  float64 // variance of last N window counts
	EnsembleScore     float64 // current ensemble score
}

// TemporalBuffer is a ring buffer tracking recent window counts for temporal analysis.
// Memory cost: 5 * 8 bytes = 40 bytes.
type TemporalBuffer struct {
	buf   [5]float64
	pos   int
	count int
}

// Push adds a new value to the ring buffer.
func (tb *TemporalBuffer) Push(v float64) {
	tb.buf[tb.pos%5] = v
	tb.pos++
	if tb.count < 5 {
		tb.count++
	}
}

// Variance returns the variance of buffered values. Returns 0 if fewer than 2 values.
func (tb *TemporalBuffer) Variance() float64 {
	if tb.count < 2 {
		return 0
	}
	var sum, sumSq float64
	for i := 0; i < tb.count; i++ {
		v := tb.buf[i]
		sum += v
		sumSq += v * v
	}
	n := float64(tb.count)
	mean := sum / n
	return sumSq/n - mean*mean
}

// Mean returns the mean of buffered values.
func (tb *TemporalBuffer) Mean() float64 {
	if tb.count == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < tb.count; i++ {
		sum += tb.buf[i]
	}
	return sum / float64(tb.count)
}

// ExtractAttackFeatures derives classification signals from window features
// and a temporal buffer. Call once per window after detection.
func ExtractAttackFeatures(f WindowFeatures, tb *TemporalBuffer, baselinePackets float64, baselineBPP float64) AttackFeatures {
	af := AttackFeatures{
		CardinalitySpike: 1.0,
		RateSpike:        1.0,
		TemporalVariance: tb.Variance(),
	}
	if f.PreviousWindowCount > 0 {
		af.CardinalitySpike = float64(f.CurrentWindowCount) / float64(f.PreviousWindowCount)
	}
	if baselinePackets > 0 {
		af.RateSpike = float64(f.PacketCount) / baselinePackets
	}
	if f.PacketCount > 0 {
		af.BytesPerPacket = float64(f.ByteVolume) / float64(f.PacketCount)
	}
	af.BaselineBPP = baselineBPP
	return af
}
