package dataclasses

import (
	"HLL-BTP/general"
	"math"
	"sync"
	"sync/atomic"
)

type ISum interface {
	ChangeSum(a, b uint8)
	GetSum() float64
	Reset()
	// Store(float64)
}

type Sum struct {
	mu  sync.Mutex
	val atomic.Uint64
}

func NewSum(f float64) *Sum {
	s := &Sum{}
	s.Store(f)
	return s
}

func (s *Sum) Store(f float64) {
	bits := math.Float64bits(f)
	s.val.Store(bits)
}

func (s *Sum) ChangeSum(a uint8, b uint8) {
	for {
		old := s.val.Load()
		oldFloat := math.Float64frombits(old)

		x := -1 * int(a)
		y := -1 * int(b)
		newFloat := oldFloat - math.Ldexp(1.0, y) + math.Ldexp(1.0, x)

		newBits := math.Float64bits(newFloat)
		if s.val.CompareAndSwap(old, newBits) {
			break
		}
	}
}

func (s *Sum) GetSum() float64 {
	bits := s.val.Load()
	return math.Float64frombits(bits)
}

func (s *Sum) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := general.ConfigPercision()
	m := 1 << p
	s.Store(float64(m))
}

type SumNonConcurrent struct {
	val float64
}

func NewSumNonConcurrent(f float64) *SumNonConcurrent {
	return &SumNonConcurrent{val: f}
}
func (s *SumNonConcurrent) ChangeSum(a, b uint8) {
	x := -1 * int(a)
	y := -1 * int(b)
	s.val -= math.Ldexp(1.0, y)
	s.val += math.Ldexp(1.0, x)
}

func (s *SumNonConcurrent) GetSum() float64 {
	return s.val
}

func (s *SumNonConcurrent) Reset() {
	p := general.ConfigPercision()
	m := 1 << p
	s.val = float64(m)
}
