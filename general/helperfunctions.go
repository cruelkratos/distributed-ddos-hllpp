package general

import (
	"encoding/json"
	"math"
	"math/bits"
	"os"
	"sync"
)

var (
	algo  string = ""
	p_val int    = -1
)

func ConfigAlgo() string {
	if algo != "" {
		return algo
	}
	data, err := os.ReadFile("config.json")
	if err != nil {
		panic(err)
	}

	// Unmarshal into a map
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		panic(err)
	}

	hashAlgo := config["hashAlgorithm"].(string)
	algo = hashAlgo
	return hashAlgo
}

func ConfigPercision() int {
	if p_val != -1 {
		return p_val
	}
	data, err := os.ReadFile("config.json")
	if err != nil {
		panic(err)
	}

	// Unmarshal into a map
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		panic(err)
	}
	precision := int(config["precision"].(float64))
	p_val = precision
	return precision
}
func Rho(w uint64, bitLength int) int {
	if bitLength <= 0 {
		return 1
	}
	// Keep only the lower (bitLength) bits
	if bitLength < 64 {
		w &= (uint64(1) << bitLength) - 1
	}
	if w == 0 {
		return bitLength + 1
	}
	// Leading zeros counted within (bitLength) window
	lz := bits.LeadingZeros64(w) - (64 - bitLength)
	return lz + 1
}

// IBucketLockManager defines the interface for our lock managers.
type IBucketLockManager interface {
	GetLockForBucket(bucketIndex int) sync.Locker
}

// BucketLockManager is the concurrent implementation.
type BucketLockManager struct {
	locks    []sync.Mutex
	numLocks int
	mask     int
}

func NewBucketLockManager(totalBuckets int) *BucketLockManager {
	var numLocks int

	switch {
	case totalBuckets <= 1024:
		numLocks = 8 // Very few locks for small HLL
	case totalBuckets <= 4096:
		numLocks = 16 // Still very memory efficient
	case totalBuckets <= 16384: // precision=14
		numLocks = 32 // Good balance
	default:
		numLocks = 64 // Cap for very large HLLs
	}

	// Round to power of 2
	actualLocks := 1
	for actualLocks < numLocks {
		actualLocks <<= 1
	}

	return &BucketLockManager{
		locks:    make([]sync.Mutex, actualLocks),
		numLocks: actualLocks,
		mask:     actualLocks - 1,
	}
}

func (blm *BucketLockManager) GetLockForBucket(bucketIndex int) sync.Locker {
	lockIndex := bucketIndex & blm.mask
	return &blm.locks[lockIndex]
}

// NoOpLockManager is the non-concurrent implementation.
type NoOpLockManager struct{}

// NoOpLock is a dummy lock that does nothing.
type noOpLock struct{}

func (l *noOpLock) Lock()   {}
func (l *noOpLock) Unlock() {}

func (m *NoOpLockManager) GetLockForBucket(bucketIndex int) sync.Locker {
	return &noOpLock{}
}

func LinearCounting(m int, V uint64) float64 {
	return float64(m) * math.Log(float64(m)/float64(V))
}
func EncodeHash(x uint64, p int, pPrime int) uint32 {
	if p >= pPrime {
		panic("p must be less than pPrime for sparse encoding")
	}
	if pPrime <= 0 || p < 0 || pPrime > 32 {
		panic("pPrime out of supported range (1..32) for uint32-packed encoding")
	}

	indexPPrime := uint32(x >> (64 - uint(pPrime)))

	// zone bits = lowest (p'-p) bits of indexPPrime
	zoneBitsShift := pPrime - p
	zoneMask := uint32(0)
	if zoneBitsShift > 0 {
		zoneMask = (uint32(1) << uint(zoneBitsShift)) - 1
	}
	zoneValue := indexPPrime & zoneMask

	if zoneValue == 0 {
		// w' = lower (64 - p') bits of x
		wPrime := x << uint(pPrime) >> uint(pPrime)
		rhoVal := uint8(Rho(wPrime, 64-pPrime))

		if rhoVal > 0x3F {
			rhoVal = 0x3F
		}
		return (indexPPrime << 7) | (uint32(rhoVal) << 1) | 1
	}

	return (indexPPrime << 1) | 0
}

func DecodeHash(k uint32, p, pPrime int) (index uint32, r uint8) {
	flag := k & 1
	if flag == 1 {
		rhoStored := uint8((k >> 1) & 0x3F)
		r = rhoStored + uint8(pPrime-p)
		index = (k >> 7) >> uint(pPrime-p)
	} else {
		// compute rho on the lower (p'-p) bits
		subbits := (k >> 1) & ((1 << (pPrime - p)) - 1)
		r = uint8(Rho(uint64(subbits), pPrime-p))
		index = (k >> 1) >> uint(pPrime-p)
	}
	return
}
