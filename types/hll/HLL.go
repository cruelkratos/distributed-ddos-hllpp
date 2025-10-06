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
	bucketLocks general.IBucketLockManager
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
	p := general.ConfigPercision()
	m := 1 << p
	alpha_m := 0.7213 / (1 + 1.079/float64(m))
	pp := p << 1
	m2 := 1 << pp
	// var temp float64 = float64(1<<32) / 30
	var E float64 = alpha_m * float64(m2) / float64(h._registers.Sum.GetSum())
	// Small range correction using Linear Counting
	if E <= 2.5*float64(m) {
		zeros := h._registers.Zeros.Get()
		if zeros != 0 {
			return uint64(general.LinearCounting(m, zeros))
		}
	}

	// For a 64-bit hash function, the large range correction for 2^32 is not needed.
	// The estimate E is returned directly for all larger cardinalities.
	// The original check `else if E <= (1<<32)/30` and the subsequent correction
	// have been removed.
	return uint64(math.Round(E))
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
func GetHLL(concurrent bool) IHLL {
	once.Do(func() {
		precision := general.ConfigPercision()
		totalBuckets := 1 << precision
		hashAlgo := general.ConfigAlgo()

		var hasher helper.IHasher
		if hashAlgo == "xxhash" {
			hasher = helper.Hasher{}
		} else {
			hasher = helper.HasherSecure{}
		}

		// Choose the lock manager based on the concurrency flag
		var lockManager general.IBucketLockManager
		if concurrent {
			lockManager = general.NewBucketLockManager(totalBuckets)
		} else {
			lockManager = &general.NoOpLockManager{}
		}

		instance = &hllSet{
			_registers:  register.NewPackedRegisters(totalBuckets, concurrent),
			helper:      hasher,
			bucketLocks: lockManager,
		}

	})
	return instance
}
