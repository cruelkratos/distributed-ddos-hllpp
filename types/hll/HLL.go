package hll

import (
	"HLL-BTP/types/register"
	"sync"
)

type IHLL interface {
	Insert(uint64)
	GetElements() uint64
	EmptySet()
}

type hllSet struct {
	mu         sync.RWMutex
	_registers *register.Registers
}

func (h *hllSet) Insert(val uint64)   {}
func (h *hllSet) GetElements() uint64 { return 1 } // main logic
func (h *hllSet) EmptySet() {
	for i := 0; i < h._registers.Size; i++ {
		h._registers.Set(i, 0)
	}

} //reset all registers

var (
	instance IHLL
	once     sync.Once
)

// Singleton HLL
func GetHLL() IHLL {
	once.Do(func() {
		instance = &hllSet{_registers: register.NewPackedRegisters(1)}
	})
	return instance
}
