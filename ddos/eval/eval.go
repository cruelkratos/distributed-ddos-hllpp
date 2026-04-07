package eval

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/window"
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"
)

// Scenario defines a detection evaluation scenario.
type Scenario struct {
	WindowDuration   time.Duration
	CheckInterval    time.Duration
	Threshold        uint64
	NormalPerWindow  int
	AttackPerWindow  int
	TotalWindows     int
	AttackWindowList []int // 0-based window indices that are "attack" (ground truth)
	Seed             int64
	// Detector overrides the default ThresholdDetector when non-nil.
	Detector detector.Detector
}

// Result holds evaluation metrics.
type Result struct {
	Recall         float64
	FalsePositives int
	FalseNegatives int
	TruePositives  int
	TrueNegatives  int
	TimeToDetect   time.Duration
	Events         []window.AttackEvent
}

// Run runs the scenario: feeds synthetic traffic into WindowManager, collects events, computes metrics.
func Run(scenario Scenario) (Result, error) {
	runStart := time.Now()
	attackSet := make(map[int]bool)
	for _, i := range scenario.AttackWindowList {
		attackSet[i] = true
	}

	det := scenario.Detector
	if det == nil {
		det = detector.NewThresholdDetector(scenario.Threshold)
	}
	eventCh := make(chan window.AttackEvent, 32)
	wm := window.NewWindowManager(scenario.WindowDuration, scenario.CheckInterval, det, eventCh)

	var events []window.AttackEvent
	var mu sync.Mutex
	// Consumer goroutine for attack events
	go func() {
		for ev := range eventCh {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}
	}()

	ipsCh := make(chan string, 50000)

	stream := &StreamSource{
		NormalIPsPerWindow: scenario.NormalPerWindow,
		AttackIPsPerWindow: scenario.AttackPerWindow,
		WindowDuration:     scenario.WindowDuration,
		TotalWindows:       scenario.TotalWindows,
		Seed:               scenario.Seed,
	}

	// Start the traffic generator
	go stream.Run(ipsCh, scenario.AttackWindowList)

	// Feed traffic into WindowManager
	for ip := range ipsCh {
		_ = wm.Insert(ip)
	}

	// Allow a small buffer for the last check to run after traffic ends
	time.Sleep(scenario.CheckInterval + 100*time.Millisecond)
	wm.Stop()
	close(eventCh)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	eventsCopy := append([]window.AttackEvent(nil), events...)
	mu.Unlock()

	// --- Metrics Calculation ---
	var firstAttackEventTime time.Time
	var tp, fp int
	attackAlerts := make(map[int]bool)

	for _, ev := range eventsCopy {
		winID := int(ev.WindowID)
		if attackSet[winID] {
			attackAlerts[winID] = true
			if firstAttackEventTime.IsZero() {
				firstAttackEventTime = ev.At
			}
			tp++
		} else {
			fp++
		}
	}

	fn := 0
	for _, i := range scenario.AttackWindowList {
		if !attackAlerts[i] {
			fn++
		}
	}

	tn := scenario.TotalWindows - len(attackSet) - fp
	if tn < 0 {
		tn = 0
	}

	recall := 0.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}

	ttd := time.Duration(0)
	if !firstAttackEventTime.IsZero() && len(scenario.AttackWindowList) > 0 {
		attackStart := runStart.Add(time.Duration(scenario.AttackWindowList[0]) * scenario.WindowDuration)
		ttd = firstAttackEventTime.Sub(attackStart)
		if ttd < 0 {
			ttd = 0
		}
	}

	return Result{
		Recall:         recall,
		FalsePositives: fp,
		FalseNegatives: fn,
		TruePositives:  tp,
		TrueNegatives:  tn,
		TimeToDetect:   ttd,
		Events:         eventsCopy,
	}, nil
}

// Run generates IPs and throttles them to match real-time window duration.
func (s *StreamSource) Run(out chan<- string, attackWindowList []int) {
	defer close(out)

	// Deterministic Randomness
	r := rand.New(rand.NewSource(s.Seed))

	isAttack := make(map[int]bool)
	for _, w := range attackWindowList {
		isAttack[w] = true
	}

	for w := 0; w < s.TotalWindows; w++ {
		windowStart := time.Now()

		// 1. Generate Normal Traffic
		for i := 0; i < s.NormalIPsPerWindow; i++ {
			out <- generateRandomIP(r)
		}

		// 2. Generate Attack Traffic (if applicable)
		if isAttack[w] {
			for i := 0; i < s.AttackIPsPerWindow; i++ {
				out <- generateRandomIP(r)
			}
		}

		// 3. Time Synchronization (Critical Fix)
		// We wait for the remainder of the window duration so the WindowManager
		// rotates to the next window at the same time we start generating the next batch.
		elapsed := time.Since(windowStart)
		if elapsed < s.WindowDuration {
			time.Sleep(s.WindowDuration - elapsed)
		}
	}
}

// generateRandomIP creates a random IPv4 string using the local random source.
func generateRandomIP(r *rand.Rand) string {
	return fmt.Sprintf("%d.%d.%d.%d", r.Intn(256), r.Intn(256), r.Intn(256), r.Intn(256))
}

// WriteCSV writes a summary and per-event CSV to path for report.
func WriteCSV(path string, scenario Scenario, res Result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	_ = w.Write([]string{"metric", "value"})
	_ = w.Write([]string{"recall", fmt.Sprintf("%.4f", res.Recall)})
	_ = w.Write([]string{"false_positives", fmt.Sprintf("%d", res.FalsePositives)})
	_ = w.Write([]string{"false_negatives", fmt.Sprintf("%d", res.FalseNegatives)})
	_ = w.Write([]string{"true_positives", fmt.Sprintf("%d", res.TruePositives)})
	_ = w.Write([]string{"time_to_detect_ns", fmt.Sprintf("%d", res.TimeToDetect.Nanoseconds())})
	_ = w.Write(nil)
	_ = w.Write([]string{"window_id", "at", "count", "reason"})
	for _, ev := range res.Events {
		_ = w.Write([]string{
			fmt.Sprintf("%d", ev.WindowID),
			ev.At.Format(time.RFC3339Nano),
			fmt.Sprintf("%d", ev.Count),
			ev.Reason,
		})
	}
	return nil
}
