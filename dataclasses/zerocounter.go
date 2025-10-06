package dataclasses

import "sync"

// IZeroCounter defines the interface for ZeroCounter operations.
type IZeroCounter interface {
	Inc()
	Dec()
	Get() uint16
}

// ZeroCounter is the thread-safe implementation.
type ZeroCounter struct {
	val uint16
	mu  sync.Mutex
}

func NewZeroCounter(vals ...uint16) *ZeroCounter {
	if len(vals) > 0 {
		return &ZeroCounter{val: vals[0]}
	}
	return &ZeroCounter{val: 0}
}

func (z *ZeroCounter) Inc() {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.val++
}

func (z *ZeroCounter) Dec() {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.val--
}

func (z *ZeroCounter) Get() uint16 {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.val
}

// ZeroCounterNonConcurrent is the non-thread-safe implementation.
type ZeroCounterNonConcurrent struct {
	val uint16
}

func NewZeroCounterNonConcurrent(vals ...uint16) *ZeroCounterNonConcurrent {
	if len(vals) > 0 {
		return &ZeroCounterNonConcurrent{val: vals[0]}
	}
	return &ZeroCounterNonConcurrent{val: 0}
}

func (z *ZeroCounterNonConcurrent) Inc() {
	z.val++
}

func (z *ZeroCounterNonConcurrent) Dec() {
	z.val--
}

func (z *ZeroCounterNonConcurrent) Get() uint16 {
	return z.val
}
