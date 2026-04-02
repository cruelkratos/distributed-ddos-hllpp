package mitigation

import (
	"hash/fnv"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	cmsRows    = 4
	cmsCols    = 512
	cmsDecayHz = 1 // decay every N seconds
)

// RateLimiter combines a global token bucket with a Count-Min Sketch for per-IP tracking.
// Memory: token bucket ~50 bytes + CMS 4×512×uint16 = 4,096 bytes ≈ 4.1 KB total.
type RateLimiter struct {
	mu sync.Mutex

	// Global token bucket.
	tokens      float64
	maxTokens   float64
	refillRate  float64 // tokens per second
	lastRefill  time.Time

	// Per-IP Count-Min Sketch.
	cms         [cmsRows][cmsCols]uint16
	perIPLimit  uint16 // max count per IP per decay window

	// Counters.
	dropped atomic.Uint64
}

// NewRateLimiter creates a rate limiter.
// globalRPS: maximum allowed requests per second globally.
// perIPLimit: maximum requests per IP per decay window.
func NewRateLimiter(globalRPS float64, perIPLimit uint16) *RateLimiter {
	if globalRPS <= 0 {
		globalRPS = 1000
	}
	if perIPLimit == 0 {
		perIPLimit = 50
	}
	return &RateLimiter{
		tokens:     globalRPS,
		maxTokens:  globalRPS * 2, // allow small burst
		refillRate: globalRPS,
		lastRefill: time.Now(),
		perIPLimit: perIPLimit,
	}
}

// refill adds tokens based on elapsed time.
func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	rl.tokens += rl.refillRate * elapsed
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now
}

// cmsHash returns CMS hash indices for an IP using FNV with per-row seeds.
func cmsHash(ip string, row int) int {
	h := fnv.New32a()
	h.Write([]byte{byte(row), byte(row >> 8)})
	h.Write([]byte(ip))
	return int(h.Sum32()) % cmsCols
}

// cmsIncrement adds 1 to the CMS for the given IP and returns the min count.
func (rl *RateLimiter) cmsIncrement(ip string) uint16 {
	var minCount uint16 = math.MaxUint16
	for r := 0; r < cmsRows; r++ {
		col := cmsHash(ip, r)
		if rl.cms[r][col] < math.MaxUint16 {
			rl.cms[r][col]++
		}
		if rl.cms[r][col] < minCount {
			minCount = rl.cms[r][col]
		}
	}
	return minCount
}

// ShouldAllow checks if the IP is within rate limits.
// Returns true if allowed, false if the request should be dropped.
func (rl *RateLimiter) ShouldAllow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	// Global rate limit check.
	if rl.tokens < 1 {
		rl.dropped.Add(1)
		return false
	}

	// Per-IP rate limit check.
	count := rl.cmsIncrement(ip)
	if count > rl.perIPLimit {
		rl.dropped.Add(1)
		return false
	}

	rl.tokens--
	return true
}

// DroppedCount returns the cumulative number of dropped requests.
func (rl *RateLimiter) DroppedCount() uint64 {
	return rl.dropped.Load()
}

// DecayCMS halves all CMS counters (call periodically to prevent saturation).
func (rl *RateLimiter) DecayCMS() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for r := 0; r < cmsRows; r++ {
		for c := 0; c < cmsCols; c++ {
			rl.cms[r][c] /= 2
		}
	}
}

// Reset clears all CMS counters and resets the token bucket.
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for r := 0; r < cmsRows; r++ {
		for c := 0; c < cmsCols; c++ {
			rl.cms[r][c] = 0
		}
	}
	rl.tokens = rl.maxTokens
	rl.lastRefill = time.Now()
}
