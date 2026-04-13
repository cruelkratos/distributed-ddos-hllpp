package eval

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ExperimentConfig captures all parameters for reproducibility.
type ExperimentConfig struct {
	ExperimentID string            `json:"experiment_id"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time,omitempty"`
	Dataset      string            `json:"dataset"`
	Detector     string            `json:"detector"`
	Nodes        int               `json:"nodes"`
	WindowDur    string            `json:"window_duration"`
	NormalRate   int               `json:"normal_rate"`
	AttackRate   int               `json:"attack_rate"`
	Scenario     string            `json:"scenario"`
	ExtraParams  map[string]string `json:"extra_params,omitempty"`
}

// TimelineEntry records per-window detection state.
type TimelineEntry struct {
	WindowID       int     `json:"window_id"`
	TimestampUnix  int64   `json:"timestamp_unix"`
	TrueLabel      string  `json:"true_label"`       // "normal" or "attack"
	PredictedLabel string  `json:"predicted_label"`   // "normal" or "attack"
	Score          float64 `json:"score"`             // ensemble/detector score
	AttackTypePred string  `json:"attack_type_pred"`  // classified attack type
	AttackTypeTrue string  `json:"attack_type_true"`  // ground-truth attack type
	UniqueIPs      uint64  `json:"unique_ips"`
	PacketCount    uint64  `json:"packet_count"`
	ByteVolume     uint64  `json:"byte_volume"`
	LatencyMs      float64 `json:"latency_ms"`        // detection latency
}

// ResourceEntry captures point-in-time resource usage.
type ResourceEntry struct {
	TimestampUnix int64   `json:"timestamp_unix"`
	Phase         string  `json:"phase"` // "normal", "attack", "recovery"
	RSSBytes      uint64  `json:"rss_bytes"`
	HeapBytes     uint64  `json:"heap_bytes"`
	StackBytes    uint64  `json:"stack_bytes"`
	CPUPercent    float64 `json:"cpu_percent"`
	Goroutines    int     `json:"goroutines"`
	GRPCBytesTx   uint64  `json:"grpc_bytes_tx"`
	GRPCBytesRx   uint64  `json:"grpc_bytes_rx"`
}

// ExperimentSummary holds aggregate metrics.
type ExperimentSummary struct {
	Detector           string  `json:"detector"`
	Dataset            string  `json:"dataset"`
	Precision          float64 `json:"precision"`
	Recall             float64 `json:"recall"`
	F1Score            float64 `json:"f1_score"`
	FalsePositiveRate  float64 `json:"false_positive_rate"`
	FalseNegativeRate  float64 `json:"false_negative_rate"`
	TruePositives      int     `json:"true_positives"`
	FalsePositives     int     `json:"false_positives"`
	TrueNegatives      int     `json:"true_negatives"`
	FalseNegatives     int     `json:"false_negatives"`
	DetectionLatencyMs float64 `json:"detection_latency_ms"`
	ClassifyAccuracy   float64 `json:"classification_accuracy"`
	AvgRSSBytes        uint64  `json:"avg_rss_bytes"`
	PeakRSSBytes       uint64  `json:"peak_rss_bytes"`
	AvgCPUPercent      float64 `json:"avg_cpu_percent"`
	TotalWindows       int     `json:"total_windows"`
}

// ExperimentLogger manages structured experiment output.
type ExperimentLogger struct {
	mu        sync.Mutex
	config    ExperimentConfig
	timeline  []TimelineEntry
	resources []ResourceEntry
	outputDir string
}

// NewExperimentLogger creates a logger that writes to the given directory.
func NewExperimentLogger(outputDir string, config ExperimentConfig) (*ExperimentLogger, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	config.StartTime = time.Now()
	if config.ExperimentID == "" {
		config.ExperimentID = fmt.Sprintf("exp_%s", time.Now().Format("20060102_150405"))
	}
	return &ExperimentLogger{
		config:    config,
		outputDir: outputDir,
	}, nil
}

// LogTimeline appends a timeline entry.
func (l *ExperimentLogger) LogTimeline(entry TimelineEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.timeline = append(l.timeline, entry)
}

// LogResource appends a resource measurement.
func (l *ExperimentLogger) LogResource(entry ResourceEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resources = append(l.resources, entry)
}

// Finish writes all output files and returns the summary.
func (l *ExperimentLogger) Finish() (ExperimentSummary, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.config.EndTime = time.Now()

	// Write config.json.
	if err := writeJSON(filepath.Join(l.outputDir, "config.json"), l.config); err != nil {
		return ExperimentSummary{}, fmt.Errorf("write config: %w", err)
	}

	// Write timeline.csv.
	if err := l.writeTimelineCSV(); err != nil {
		return ExperimentSummary{}, fmt.Errorf("write timeline: %w", err)
	}

	// Write metrics.csv (resources).
	if err := l.writeResourceCSV(); err != nil {
		return ExperimentSummary{}, fmt.Errorf("write metrics: %w", err)
	}

	// Compute summary.
	summary := l.computeSummary()

	// Write summary.json.
	if err := writeJSON(filepath.Join(l.outputDir, "summary.json"), summary); err != nil {
		return ExperimentSummary{}, fmt.Errorf("write summary: %w", err)
	}

	return summary, nil
}

func (l *ExperimentLogger) computeSummary() ExperimentSummary {
	s := ExperimentSummary{
		Detector:     l.config.Detector,
		Dataset:      l.config.Dataset,
		TotalWindows: len(l.timeline),
	}

	classifyCorrect := 0
	classifyTotal := 0
	var totalLatency float64
	latencyCount := 0

	for _, t := range l.timeline {
		isAttack := t.TrueLabel == "attack"
		predicted := t.PredictedLabel == "attack"

		if isAttack && predicted {
			s.TruePositives++
		} else if !isAttack && predicted {
			s.FalsePositives++
		} else if isAttack && !predicted {
			s.FalseNegatives++
		} else {
			s.TrueNegatives++
		}

		if t.LatencyMs > 0 && isAttack && predicted {
			totalLatency += t.LatencyMs
			latencyCount++
		}

		if t.AttackTypeTrue != "" && t.AttackTypePred != "" {
			classifyTotal++
			if t.AttackTypeTrue == t.AttackTypePred {
				classifyCorrect++
			}
		}
	}

	if s.TruePositives+s.FalseNegatives > 0 {
		s.Recall = float64(s.TruePositives) / float64(s.TruePositives+s.FalseNegatives)
	}
	if s.TruePositives+s.FalsePositives > 0 {
		s.Precision = float64(s.TruePositives) / float64(s.TruePositives+s.FalsePositives)
	}
	if s.Precision+s.Recall > 0 {
		s.F1Score = 2 * s.Precision * s.Recall / (s.Precision + s.Recall)
	}
	if s.FalsePositives+s.TrueNegatives > 0 {
		s.FalsePositiveRate = float64(s.FalsePositives) / float64(s.FalsePositives+s.TrueNegatives)
	}
	if s.FalseNegatives+s.TruePositives > 0 {
		s.FalseNegativeRate = float64(s.FalseNegatives) / float64(s.FalseNegatives+s.TruePositives)
	}
	if latencyCount > 0 {
		s.DetectionLatencyMs = totalLatency / float64(latencyCount)
	}
	if classifyTotal > 0 {
		s.ClassifyAccuracy = float64(classifyCorrect) / float64(classifyTotal)
	}

	// Resource aggregates.
	var totalRSS, totalCPU float64
	for _, r := range l.resources {
		totalRSS += float64(r.RSSBytes)
		totalCPU += r.CPUPercent
		if r.RSSBytes > s.PeakRSSBytes {
			s.PeakRSSBytes = r.RSSBytes
		}
	}
	if len(l.resources) > 0 {
		s.AvgRSSBytes = uint64(totalRSS / float64(len(l.resources)))
		s.AvgCPUPercent = totalCPU / float64(len(l.resources))
	}

	return s
}

func (l *ExperimentLogger) writeTimelineCSV() error {
	f, err := os.Create(filepath.Join(l.outputDir, "timeline.csv"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"window_id", "timestamp", "true_label", "predicted_label", "score",
		"attack_type_true", "attack_type_pred", "unique_ips", "packet_count", "byte_volume", "latency_ms"})
	for _, t := range l.timeline {
		w.Write([]string{
			fmt.Sprintf("%d", t.WindowID),
			fmt.Sprintf("%d", t.TimestampUnix),
			t.TrueLabel,
			t.PredictedLabel,
			fmt.Sprintf("%.4f", t.Score),
			t.AttackTypeTrue,
			t.AttackTypePred,
			fmt.Sprintf("%d", t.UniqueIPs),
			fmt.Sprintf("%d", t.PacketCount),
			fmt.Sprintf("%d", t.ByteVolume),
			fmt.Sprintf("%.2f", t.LatencyMs),
		})
	}
	return nil
}

func (l *ExperimentLogger) writeResourceCSV() error {
	f, err := os.Create(filepath.Join(l.outputDir, "metrics.csv"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"timestamp", "phase", "rss_bytes", "heap_bytes", "stack_bytes",
		"cpu_percent", "goroutines", "grpc_bytes_tx", "grpc_bytes_rx"})
	for _, r := range l.resources {
		w.Write([]string{
			fmt.Sprintf("%d", r.TimestampUnix),
			r.Phase,
			fmt.Sprintf("%d", r.RSSBytes),
			fmt.Sprintf("%d", r.HeapBytes),
			fmt.Sprintf("%d", r.StackBytes),
			fmt.Sprintf("%.2f", r.CPUPercent),
			fmt.Sprintf("%d", r.Goroutines),
			fmt.Sprintf("%d", r.GRPCBytesTx),
			fmt.Sprintf("%d", r.GRPCBytesRx),
		})
	}
	return nil
}

func writeJSON(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
