package hll

import "sync"

type IHLL interface {
	Insert(uint64)
	GetElements() uint64
	EmptySet()
}

type hllSet struct {
	hi uint32
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
		instance = hllSet{hi: 1}
	})
	return instance
}
