package hll

import (
	"HLL-BTP/general"
	"HLL-BTP/types/register"
	"HLL-BTP/types/register/helper"
	"math"
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
func (h *hllSet) GetElements() uint64 {
	alpha_m := 0.7213 / (1 + 1.079/(1<<14))
	p := general.ConfigPercision()
	pp := p << 1
	m := 1 << p
	m2 := 1 << pp
	var temp float64 = float64(1<<32) / 30
	var E float64 = alpha_m * float64(m2) / float64(h._registers.Sum.GetSum())
	if E <= float64(m)*2.5 {
		V := h._registers.Zeros.Get()
		if h._registers.Zeros.Get() != 0 {
			return uint64(math.Round(general.LinearCounting(m, V)))
		}
		return uint64(math.Round(E))
	} else if E <= temp {
		return uint64(math.Round(E))
	} else {
		two32 := float64(uint64(1) << 32)
		return uint64(math.Round(-two32 * math.Log(1.0-(E/two32))))
	}
}
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
