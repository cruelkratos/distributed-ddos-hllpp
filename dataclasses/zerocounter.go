package dataclasses

import "sync"

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
