package eval

import (
	"time"
)

// StreamSource produces IPs over time for evaluation. It implements a scenario
// with normal and attack periods (ground truth).
type StreamSource struct {
	// NormalIPsPerWindow is the number of distinct IPs to inject per window during normal periods.
	NormalIPsPerWindow int
	// AttackIPsPerWindow is the number of distinct IPs to inject per window during attack periods.
	AttackIPsPerWindow int
	// WindowDuration is the duration of one logical window (used to spread IPs).
	WindowDuration time.Duration
	// TotalWindows is the number of windows to simulate.
	TotalWindows int
	// Seed for reproducible IPs (optional).
	Seed int64
}

// Run runs the scenario: for each window, pushes IPs to ips and advances time.
// It closes ips when done. attackWindowIndices lists which window indices are attack (ground truth).
