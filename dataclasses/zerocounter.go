package dataclasses

import (
	"HLL-BTP/general"
	"sync"
)

// IZeroCounter defines the interface for ZeroCounter operations.
type IZeroCounter interface {
	Inc()
	Dec()
	Get() uint16
	Reset()
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

func (z *ZeroCounter) Reset() {
	z.mu.Lock()
	defer z.mu.Unlock()
	p := general.ConfigPercision()
	m := 1 << p
	z.val = uint16(m)
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

func (z *ZeroCounterNonConcurrent) Reset() {
	p := general.ConfigPercision()
	m := 1 << p
	z.val = uint16(m)
}
