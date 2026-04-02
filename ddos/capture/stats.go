package capture

import "sync/atomic"

// TrafficStats tracks packet count and byte volume with atomic operations
// for lock-free concurrent updates from the capture hot-path.
type TrafficStats struct {
	packets atomic.Uint64
	bytes   atomic.Uint64
}

// RecordPacket adds one packet with the given byte length.
func (s *TrafficStats) RecordPacket(byteLen uint64) {
	s.packets.Add(1)
	s.bytes.Add(byteLen)
}

// Snapshot returns current counters and resets them atomically.
func (s *TrafficStats) Snapshot() (packets, bytes uint64) {
	packets = s.packets.Swap(0)
	bytes = s.bytes.Swap(0)
	return
}

// Packets returns the current packet count without resetting.
func (s *TrafficStats) Packets() uint64 { return s.packets.Load() }

// Bytes returns the current byte count without resetting.
func (s *TrafficStats) Bytes() uint64 { return s.bytes.Load() }
