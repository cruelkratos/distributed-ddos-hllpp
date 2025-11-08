package dataclasses

import (
	"HLL-BTP/general"
	"sync"
	"sync/atomic"
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
	val atomic.Uint32
	mu  sync.Mutex
}

func NewZeroCounter(vals ...uint16) *ZeroCounter {
	if len(vals) > 0 {
		z := &ZeroCounter{}
		z.Store(vals[0])
		return z
	}
	z := &ZeroCounter{}
	z.Store(0)
	return z
}

func (z *ZeroCounter) Store(v uint16) {
	z.val.Store(uint32(v))

}

func (z *ZeroCounter) Inc() {
	z.val.Add(1)
}

func (z *ZeroCounter) Dec() {
	z.val.Add(^uint32(0))
}

func (z *ZeroCounter) Get() uint16 {
	return uint16(z.val.Load())
}

func (z *ZeroCounter) Reset() {
	z.mu.Lock()
	defer z.mu.Unlock()

	p := general.ConfigPercision()
	m := 1 << p
	z.val.Store(uint32(m))
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
