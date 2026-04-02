package detector

// WindowFeatures holds inputs for detection (threshold or ML).
type WindowFeatures struct {
	CurrentWindowCount  uint64
	PreviousWindowCount uint64
	WindowDurationSec   float64

	// Extended fields for ML-based detection.
	PacketCount   uint64  // total packets in current window
	ByteVolume    uint64  // total bytes in current window
	EWMAResidual  float64 // deviation from EWMA baseline (current - ewma) / ewma
	ZScoreValue   float64 // z-score of current window count
}

// FeatureVector is the 8-dimension numeric input to LODA/HST detectors.
// Order: [uniqueIPs, prevUniqueIPs, ipsRatio, packetCount, byteVolume, bytesPerPacket, ewmaResidual, zScore]
type FeatureVector [8]float64

// ExtractFeatures converts WindowFeatures into a normalized FeatureVector.
func ExtractFeatures(f WindowFeatures) FeatureVector {
	var fv FeatureVector
	fv[0] = float64(f.CurrentWindowCount)
	fv[1] = float64(f.PreviousWindowCount)
	if f.PreviousWindowCount > 0 {
		fv[2] = float64(f.CurrentWindowCount) / float64(f.PreviousWindowCount)
	} else {
		fv[2] = 1.0
	}
	fv[3] = float64(f.PacketCount)
	fv[4] = float64(f.ByteVolume)
	if f.PacketCount > 0 {
		fv[5] = float64(f.ByteVolume) / float64(f.PacketCount)
	}
	fv[6] = f.EWMAResidual
	fv[7] = f.ZScoreValue
	return fv
}

// Detector decides if the current window indicates a DDoS attack.
type Detector interface {
	IsAttack(f WindowFeatures) bool
	Name() string // e.g. "threshold", "zscore_anomaly", "ensemble"
}

// ScoringDetector extends Detector with a numeric score output for telemetry.
type ScoringDetector interface {
	Detector
	Score(f WindowFeatures) float64
}
