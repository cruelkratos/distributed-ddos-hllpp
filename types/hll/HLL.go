package hll

import (
	"HLL-BTP/general"
	"HLL-BTP/models"
	"HLL-BTP/types/register"
	"HLL-BTP/types/register/helper"
	"math"
	"sync"
)

type IHLL interface {
	Insert(string)
	GetElements() uint64
	GetRawEstimate() float64
	EmptySet()
	Reset() // Added for benchmarking
}

type hllSet struct {
	bucketLocks             general.IBucketLockManager
	_registers              *register.Registers
	helper                  helper.IHasher
	algorithm               string
	useLargeRangeCorrection bool
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

func (h *hllSet) GetRawEstimate() float64 {
	p := general.ConfigPercision()
	m := 1 << p
	alpha_m := 0.7213 / (1 + 1.079/float64(m))
	return alpha_m * float64(m*m) / h._registers.Sum.GetSum()
}

func (h *hllSet) GetElements() uint64 {
	p := general.ConfigPercision()
	m := 1 << p
	alpha_m := 0.7213 / (1 + 1.079/float64(m))

	rawEstimate := alpha_m * float64(m*m) / h._registers.Sum.GetSum()

	if h.algorithm == "hllpp" {
		// --- HLL++ LOGIC (CORRECTED TO MATCH PAPER) ---

		// Line 37: Calculate E', the bias-corrected estimate.
		// The correction is only applied if the raw estimate is within the valid range for the bias data.
		biasCorrectedEstimate := rawEstimate
		if rawEstimate <= float64(5*m) {
			biasCorrectedEstimate = rawEstimate - getbiascorrector().getCorrection(rawEstimate)
		}

		// Lines 38-43: Calculate H, the LinearCounting estimate.
		// If there are no zero registers, H is simply the bias-corrected estimate.
		var linearCountingEstimate float64
		zeros := h._registers.Zeros.Get()
		if zeros > 0 {
			linearCountingEstimate = general.LinearCounting(m, zeros)
		} else {
			// When no registers are zero, LinearCounting is invalid.
			// The pseudocode implies we use the corrected estimate E' in this case.
			linearCountingEstimate = biasCorrectedEstimate
		}

		// Use the LinearCounting result to decide.
		threshold := float64(models.HLLPlusPlusThresholds[p])
		if linearCountingEstimate <= threshold {
			// If H is below the threshold, it is the more accurate estimate.
			return uint64(math.Round(linearCountingEstimate))
		} else {
			// Otherwise, the bias-corrected estimate E' is the one to use.
			return uint64(math.Round(biasCorrectedEstimate))
		}
		// --- END OF CORRECTION ---
	}

	// --- Original HLL Logic ---
	zeros := h._registers.Zeros.Get()
	if rawEstimate <= 2.5*float64(m) {
		if zeros != 0 {
			return uint64(general.LinearCounting(m, zeros))
		}
		// Fall through to return rawEstimate if zeros is 0 but rawEstimate is low
	}

	if h.useLargeRangeCorrection && rawEstimate > (1<<32)/30.0 {
		two32 := float64(uint64(1) << 32)
		return uint64(math.Round(-two32 * math.Log(1.0-(rawEstimate/two32))))
	}

	return uint64(math.Round(rawEstimate))
}

func (h *hllSet) EmptySet() {
	for i := 0; i < h._registers.Size; i++ {
		h._registers.Set(i, 0)
	}
}

// Reset clears the singleton instance for new benchmark runs.
func (h *hllSet) Reset() {
	once = sync.Once{}
	instance = nil
}

var (
	instance IHLL
	once     sync.Once
)

// Singleton HLL
func GetHLL(concurrent bool, algorithm string, useLargeRangeCorrection bool) IHLL {
	// This check is for the benchmark reset.
	// It's not standard for singletons but necessary here.
	if instance != nil {
		// If flags have changed, we need a new instance.
		h, ok := instance.(*hllSet)
		if !ok || h.algorithm != algorithm || h.useLargeRangeCorrection != useLargeRangeCorrection {
			instance = nil // Force re-creation
		}
	}

	if instance == nil {
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

			var lockManager general.IBucketLockManager
			if concurrent {
				lockManager = general.NewBucketLockManager(totalBuckets)
			} else {
				lockManager = &general.NoOpLockManager{}
			}

			instance = &hllSet{
				_registers:              register.NewPackedRegisters(totalBuckets, concurrent),
				helper:                  hasher,
				bucketLocks:             lockManager,
				algorithm:               algorithm,
				useLargeRangeCorrection: useLargeRangeCorrection,
			}
		})
	}
	return instance
}
