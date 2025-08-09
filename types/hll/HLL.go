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
	_registers register.Registers
}

func (h hllSet) Insert(val uint64)   {}
func (h hllSet) GetElements() uint64 { return 1 }
func (h hllSet) EmptySet()           {}

var (
	instance IHLL
	once     sync.Once
)

func GetHLL() IHLL {
	once.Do(func() {
		instance = hllSet{_registers: *register.NewPackedRegisters(1)}
	})
	return instance
}
