package dataclasses

import (
	"HLL-BTP/general"
	"sync/atomic"
)

// IZeroCounter defines the interface for ZeroCounter operations.
type IZeroCounter interface {
	Inc()
	Dec()
	Get() uint16
	Reset()
	Store(uint16) // Added for Recalculate
}

// ZeroCounter is the thread-safe implementation.
type ZeroCounter struct {
	val atomic.Uint32
}

func NewZeroCounter(vals ...uint16) *ZeroCounter {
	z := &ZeroCounter{}
	if len(vals) > 0 {
		z.Store(vals[0])
	} else {
		z.Store(0)
	}
	return z
}

func (z *ZeroCounter) Store(v uint16) {
	z.val.Store(uint32(v))
}

func (z *ZeroCounter) Inc() {
	z.val.Add(1)
}

func (z *ZeroCounter) Dec() {
	z.val.Add(^uint32(0)) // Atomic decrement
}

func (z *ZeroCounter) Get() uint16 {
	return uint16(z.val.Load())
}

func (z *ZeroCounter) Reset() {
	// Reset logic already implemented by you
	p := general.ConfigPercision()
	m := 1 << p
	z.Store(uint16(m))
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

// Store implementation for non-concurrent version
func (z *ZeroCounterNonConcurrent) Store(v uint16) {
	z.val = v
}
