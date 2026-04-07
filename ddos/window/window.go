package window

import (
	"HLL-BTP/ddos/capture"
	"HLL-BTP/ddos/detector"
	"HLL-BTP/general"
	pb "HLL-BTP/server"
	"HLL-BTP/types/hll"
	"sync"
	"sync/atomic"
	"time"
)

// AttackEvent is emitted when the detector signals an attack.
type AttackEvent struct {
	At       time.Time
	Count    uint64
	WindowID int64
	Reason   string
}

// WindowManager manages current and previous HLL++ sketches and rotates them every windowDur.
// Detection is done via the pluggable Detector; when it signals attack, an AttackEvent is sent to onAttack (if set).
type WindowManager struct {
	current   *hll.Hllpp_set
	previous  *hll.Hllpp_set
	mu        sync.RWMutex
	windowDur time.Duration
	checkDur  time.Duration
	detector  detector.Detector
	onAttack  chan<- AttackEvent
	stop      chan struct{}
	wg        sync.WaitGroup
	windowID  int64

	// Extended traffic stats for ML features.
	Stats     *capture.TrafficStats
	prevStats struct {
		packets uint64
		bytes   uint64
	}
	// Last computed features (for telemetry export).
	lastFeatures atomic.Value // stores detector.WindowFeatures
}

// NewWindowManager creates a WindowManager with two HLL++ sketches, starts the rotation goroutine
// and the detection-check goroutine. If onAttack is non-nil, attack events are sent there.
func NewWindowManager(windowDur, checkInterval time.Duration, det detector.Detector, onAttack chan<- AttackEvent) *WindowManager {
	if checkInterval <= 0 {
		checkInterval = time.Second
	}
	wm := &WindowManager{
		current:   hll.GetHLLPP(true),
		previous:  hll.GetHLLPP(true),
		windowDur: windowDur,
		checkDur:  checkInterval,
		detector:  det,
		onAttack:  onAttack,
		stop:      make(chan struct{}),
		Stats:     &capture.TrafficStats{},
	}
	wm.wg.Add(1)
	go wm.rotationLoop()
	wm.wg.Add(1)
	go wm.checkLoop()
	return wm
}

func (wm *WindowManager) rotationLoop() {
	defer wm.wg.Done()
	ticker := time.NewTicker(wm.windowDur)
	defer ticker.Stop()
	for {
		select {
		case <-wm.stop:
			return
		case <-ticker.C:
			wm.rotate()
		}
	}
}

func (wm *WindowManager) rotate() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	// Swap: current -> previous, previous -> current; then reset (new) current.
	wm.current, wm.previous = wm.previous, wm.current
	wm.current.Reset()
	atomic.AddInt64(&wm.windowID, 1)

	// Snapshot traffic stats for the completed window and reset counters.
	p, b := wm.Stats.Snapshot()
	wm.prevStats.packets = p
	wm.prevStats.bytes = b
}

func (wm *WindowManager) checkLoop() {
	defer wm.wg.Done()
	ticker := time.NewTicker(wm.checkDur)
	defer ticker.Stop()
	for {
		select {
		case <-wm.stop:
			return
		case <-ticker.C:
			wm.runCheck()
		}
	}
}

func (wm *WindowManager) runCheck() {
	current := wm.CurrentCount()
	previous := wm.PreviousCount()
	windowSec := wm.windowDur.Seconds()

	wm.mu.RLock()
	pkts := wm.prevStats.packets
	byts := wm.prevStats.bytes
	wm.mu.RUnlock()

	features := detector.WindowFeatures{
		CurrentWindowCount:  current,
		PreviousWindowCount: previous,
		WindowDurationSec:   windowSec,
		PacketCount:         pkts,
		ByteVolume:          byts,
	}
	wm.lastFeatures.Store(features)
	// Detection is handled by the agent's goroutine to avoid double-updating
	// stateful sub-detectors (ZScore history, EWMA baseline).
}

// LastFeatures returns the most recently computed WindowFeatures (for telemetry).
func (wm *WindowManager) LastFeatures() detector.WindowFeatures {
	v := wm.lastFeatures.Load()
	if v == nil {
		return detector.WindowFeatures{}
	}
	return v.(detector.WindowFeatures)
}

// Insert adds the IP into the current window's sketch.
func (wm *WindowManager) Insert(ip string) error {
	wm.mu.RLock()
	cur := wm.current
	wm.mu.RUnlock()
	if cur == nil {
		return nil
	}
	return cur.Insert(ip)
}

// CurrentCount returns the distinct count estimate for the current window.
func (wm *WindowManager) CurrentCount() uint64 {
	wm.mu.RLock()
	cur := wm.current
	wm.mu.RUnlock()
	if cur == nil {
		return 0
	}
	return cur.GetElements()
}

// PreviousCount returns the distinct count estimate for the previous (last completed) window.
func (wm *WindowManager) PreviousCount() uint64 {
	wm.mu.RLock()
	prev := wm.previous
	wm.mu.RUnlock()
	if prev == nil {
		return 0
	}
	return prev.GetElements()
}

// ApproxMemoryBytes returns approximate memory used by both HLL sketches (current + previous).
// Uses theoretical dense size for p=14: 2 * (m*6+7)/8.
func (wm *WindowManager) ApproxMemoryBytes() uint64 {
	p := general.ConfigPercision()
	m := 1 << p
	denseBytes := (m*6 + 7) / 8
	return uint64(2 * denseBytes)
}

// ExportCurrentSketch serializes the current window's HLL++ sketch for gRPC shipping.
func (wm *WindowManager) ExportCurrentSketch() (*pb.Sketch, error) {
	wm.mu.RLock()
	cur := wm.current
	wm.mu.RUnlock()
	if cur == nil {
		return nil, nil
	}
	return cur.ExportSketch()
}

// Stop stops the rotation and check goroutines. Call once before exit.
func (wm *WindowManager) Stop() {
	close(wm.stop)
	wm.wg.Wait()
}
