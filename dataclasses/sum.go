package dataclasses

import "sync"

type Sum struct {
	mu  sync.RWMutex
	val uint64
}

func (s *Sum) ChangeSum(a uint8, b uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.val -= (1 << b)
	s.val += (1 << a)
}

func (s *Sum) GetSum() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.val
}
