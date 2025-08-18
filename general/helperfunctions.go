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

func (blm *BucketLockManager) GetLockForBucket(bucketIndex int) *sync.Mutex {
	lockIndex := bucketIndex & blm.mask
	return &blm.locks[lockIndex]
}

func LinearCounting(m int, V uint16) float64 {
	return float64(m) * math.Log(float64(m)/float64(V))
}
