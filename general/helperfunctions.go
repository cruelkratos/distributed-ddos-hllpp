package general

import (
	"encoding/json"
	"math/bits"
	"os"
	"sync"
)

func ConfigAlgo() string {
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
	return hashAlgo
}

func ConfigPercision() int {
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
	return precision
}
func Rho(w uint64, bitLength int) int {
	if w == 0 {
		return bitLength + 1
	}
	leadingZeros := bits.LeadingZeros64(w) // counts from MSB
	return leadingZeros + 1
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
