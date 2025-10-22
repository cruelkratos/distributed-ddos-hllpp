package dataclasses

import (
	"HLL-BTP/general"
	"math"
	"sync"
)

type ISum interface {
	ChangeSum(a, b uint8)
	GetSum() float64
	Reset()
}

type Sum struct {
	mu  sync.Mutex
	val float64
}

func NewSum(f float64) *Sum {
	return &Sum{val: f}
}

func (s *Sum) ChangeSum(a uint8, b uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := -1 * int(a)
	y := -1 * int(b)
	s.val -= math.Ldexp(1.0, y)
	s.val += math.Ldexp(1.0, x)
}

func (s *Sum) GetSum() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.val
}

func (s *Sum) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := general.ConfigPercision()
	m := 1 << p
	s.val = float64(m)
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
