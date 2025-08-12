package hll

import (
	"HLL-BTP/general"
	"HLL-BTP/types/register"
	"HLL-BTP/types/register/helper"
	"sync"
)

type IHLL interface {
	Insert(string)
	GetElements() uint64
	EmptySet()
}

type hllSet struct {
	bucketLocks *general.BucketLockManager
	_registers  *register.Registers
	helper      helper.IHasher
}

func (h *hllSet) Insert(ip string) {
	hash := h.helper.HashIP(ip)
	p := general.ConfigPercision()
	idx := hash >> (64 - p)
	w := hash << p
	w = w >> p
	r := general.Rho(w, 64-p)

	lock := h.bucketLocks.GetLockForBucket(int(idx))
	lock.Lock()
	defer lock.Unlock()

	v := h._registers.Get(int(idx))
	h._registers.Set(int(idx), max(v, uint8(r)))
}
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
		precision := general.ConfigPercision()
		totalBuckets := 1 << precision
		hashAlgo := general.ConfigAlgo()
		if hashAlgo == "xxhash" {
			instance = &hllSet{_registers: register.NewPackedRegisters(totalBuckets), helper: helper.Hasher{},
				bucketLocks: general.NewBucketLockManager(totalBuckets)}
		} else {
			instance = &hllSet{_registers: register.NewPackedRegisters(totalBuckets), helper: helper.HasherSecure{},
				bucketLocks: general.NewBucketLockManager(totalBuckets)}
		}
	})
	return instance
}
