// eval-detection runs the DDoS detection evaluation scenario and outputs metrics for the report.
// Supports synthetic traffic, all detectors (threshold, zscore, ewma, ensemble, loda, hst),
// compare mode, ablation studies, and structured experiment output via ExperimentLogger.
package main

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/eval"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"
)

func main() {
	windowDur := flag.Duration("window", 2*time.Second, "Window duration.")
	threshold := flag.Uint64("threshold", 5000, "Threshold for attack (used by threshold detector).")
	normalIPs := flag.Int("normal", 300, "Distinct IPs per window during normal.")
	attackIPs := flag.Int("attack", 15000, "Distinct IPs per window during attack.")
	totalWindows := flag.Int("windows", 12, "Total number of windows.")
	attackFrom := flag.Int("attack-from", 4, "First attack window index (0-based).")
	attackCount := flag.Int("attack-count", 4, "Number of consecutive attack windows.")
	seed := flag.Int64("seed", 42, "RNG seed for reproducibility.")
	outCSV := flag.String("out", "", "Optional: write results CSV to this path.")
	detectorName := flag.String("detector", "threshold", "Detector: threshold | zscore | ewma | ensemble | loda | hst | compare | ablation")
	// Z-score flags
	zsHistory := flag.Int("zs-history", 20, "Z-score: rolling history length.")
	zsThreshold := flag.Float64("zs-threshold", 3.0, "Z-score: sigma threshold to trigger alert.")
	// EWMA flags
	ewmaAlpha := flag.Float64("ewma-alpha", 0.2, "EWMA: smoothing factor (0<α≤1).")
	ewmaDeviation := flag.Float64("ewma-deviation", 2.0, "EWMA: proportional deviation above baseline (e.g. 2.0 = 200%).")
	ewmaWarmup := flag.Int("ewma-warmup", 5, "EWMA: warmup windows before detection starts.")
	// Experiment logging flags
	experimentDir := flag.String("experiment-dir", "", "Directory for structured experiment output (config.json, timeline.csv, summary.json).")
	experimentID := flag.String("experiment-id", "", "Experiment ID for logger. Auto-generated if empty.")
	flag.Parse()

	attackList := make([]int, 0, *attackCount)
	for i := 0; i < *attackCount; i++ {
		attackList = append(attackList, *attackFrom+i)
	}

	baseScenario := eval.Scenario{
		WindowDuration:   *windowDur,
		CheckInterval:    time.Second,
		Threshold:        *threshold,
		NormalPerWindow:  *normalIPs,
		AttackPerWindow:  *attackIPs,
		TotalWindows:     *totalWindows,
		AttackWindowList: attackList,
		Seed:             *seed,
	}

	buildDetector := func(name string) detector.Detector {
		switch name {
		case "zscore":
			return detector.NewZScoreDetector(*zsHistory, *zsThreshold)
		case "ewma":
			return detector.NewEWMADetector(*ewmaAlpha, *ewmaDeviation, *ewmaWarmup)
		case "ensemble":
			return detector.NewEnsembleDetector(42, 0.6, detector.DefaultEnsembleWeights())
		case "loda":
			// LODA/HST only work via ensemble scoring — use ensemble with heavier LODA weight.
			w := detector.DefaultEnsembleWeights()
			w.LODA = 0.8
			w.HST = 0.0
			return detector.NewEnsembleDetector(42, 0.6, w)
		case "hst":
			w := detector.DefaultEnsembleWeights()
			w.HST = 0.8
			w.LODA = 0.0
			return detector.NewEnsembleDetector(42, 0.6, w)
		default: // "threshold"
			return nil // eval.Run uses ThresholdDetector when Detector is nil
		}
	}

	// --- Ablation mode: run each sub-detector and combinations ---
	if *detectorName == "ablation" {
		names := []string{"threshold", "zscore", "ewma", "loda", "hst", "ensemble"}
		fmt.Printf("%-12s  %8s  %8s  %8s  %6s  %4s  %4s  %4s  %4s  %10s\n",
			"detector", "recall", "prec", "f1", "fpr", "TP", "FP", "FN", "TN", "ttd")
		fmt.Println("--------------------------------------------------------------------------------------------")
		for _, name := range names {
			sc := baseScenario
			sc.Detector = buildDetector(name)
			res, err := eval.Run(sc)
			if err != nil {
				log.Fatalf("eval(%s): %v", name, err)
			}
			prec, f1, fpr := computeExtended(res)

			// Write per-detector experiment output if dir specified.
			if *experimentDir != "" {
				writeExperimentOutput(filepath.Join(*experimentDir, name), name, res, baseScenario, *experimentID)
			}

			fmt.Printf("%-12s  %8.4f  %8.4f  %8.4f  %6.4f  %4d  %4d  %4d  %4d  %10s\n",
				name, res.Recall, prec, f1, fpr,
				res.TruePositives, res.FalsePositives,
				res.FalseNegatives, res.TrueNegatives, res.TimeToDetect)
		}
		return
	}

	// --- Compare mode: all detectors side by side ---
	if *detectorName == "compare" {
		names := []string{"threshold", "zscore", "ewma", "ensemble"}
		fmt.Printf("%-12s  %8s  %8s  %8s  %6s  %4s  %4s  %4s  %4s  %10s\n",
			"detector", "recall", "prec", "f1", "fpr", "TP", "FP", "FN", "TN", "ttd")
		fmt.Println("--------------------------------------------------------------------------------------------")
		for _, name := range names {
			sc := baseScenario
			sc.Detector = buildDetector(name)
			res, err := eval.Run(sc)
			if err != nil {
				log.Fatalf("eval(%s): %v", name, err)
			}
			prec, f1, fpr := computeExtended(res)
			fmt.Printf("%-12s  %8.4f  %8.4f  %8.4f  %6.4f  %4d  %4d  %4d  %4d  %10s\n",
				name, res.Recall, prec, f1, fpr,
				res.TruePositives, res.FalsePositives,
				res.FalseNegatives, res.TrueNegatives, res.TimeToDetect)
		}
		return
	}

	// --- Single detector run ---
	scenario := baseScenario
	scenario.Detector = buildDetector(*detectorName)

	log.Printf("Running scenario: detector=%s window=%s threshold=%d normal=%d attack=%d windows=%d attack_windows=%v",
		*detectorName, *windowDur, *threshold, *normalIPs, *attackIPs, *totalWindows, attackList)

	res, err := eval.Run(scenario)
	if err != nil {
		log.Fatalf("eval: %v", err)
	}

	prec, f1, fpr := computeExtended(res)

	fmt.Println("--- Detection evaluation results ---")
	fmt.Printf("Detector:          %s\n", *detectorName)
	fmt.Printf("Recall:            %.4f\n", res.Recall)
	fmt.Printf("Precision:         %.4f\n", prec)
	fmt.Printf("F1 Score:          %.4f\n", f1)
	fmt.Printf("FPR:               %.4f\n", fpr)
	fmt.Printf("False positives:   %d\n", res.FalsePositives)
	fmt.Printf("False negatives:   %d\n", res.FalseNegatives)
	fmt.Printf("True positives:    %d\n", res.TruePositives)
	fmt.Printf("True negatives:    %d\n", res.TrueNegatives)
	fmt.Printf("Time to detect:    %s\n", res.TimeToDetect)
	fmt.Printf("Total events:      %d\n", len(res.Events))

	if *outCSV != "" {
		if err := eval.WriteCSV(*outCSV, scenario, res); err != nil {
			log.Fatalf("write CSV: %v", err)
		}
		log.Printf("Wrote %s", *outCSV)
	}

	if *experimentDir != "" {
		writeExperimentOutput(*experimentDir, *detectorName, res, scenario, *experimentID)
	}
}

// computeExtended derives precision, F1, and FPR from eval.Result.
func computeExtended(res eval.Result) (precision, f1, fpr float64) {
	if res.TruePositives+res.FalsePositives > 0 {
		precision = float64(res.TruePositives) / float64(res.TruePositives+res.FalsePositives)
	}
	if precision+res.Recall > 0 {
		f1 = 2 * precision * res.Recall / (precision + res.Recall)
	}
	if res.FalsePositives+res.TrueNegatives > 0 {
		fpr = float64(res.FalsePositives) / float64(res.FalsePositives+res.TrueNegatives)
	}
	return
}

// writeExperimentOutput creates structured experiment output using the logger.
func writeExperimentOutput(dir, detName string, res eval.Result, sc eval.Scenario, expID string) {
	config := eval.ExperimentConfig{
		ExperimentID: expID,
		Detector:     detName,
		Dataset:      "synthetic",
		WindowDur:    sc.WindowDuration.String(),
		NormalRate:   sc.NormalPerWindow,
		AttackRate:   sc.AttackPerWindow,
		Scenario:     fmt.Sprintf("windows=%d attack=%v", sc.TotalWindows, sc.AttackWindowList),
	}

	logger, err := eval.NewExperimentLogger(dir, config)
	if err != nil {
		log.Printf("experiment logger: %v", err)
		return
	}

	// Build attack set for labeling.
	attackSet := make(map[int]bool)
	for _, w := range sc.AttackWindowList {
		attackSet[w] = true
	}

	// Record timeline from events.
	alertedWindows := make(map[int]bool)
	for _, ev := range res.Events {
		alertedWindows[int(ev.WindowID)] = true
	}
	for w := 0; w < sc.TotalWindows; w++ {
		trueLabel := "normal"
		if attackSet[w] {
			trueLabel = "attack"
		}
		predicted := "normal"
		if alertedWindows[w] {
			predicted = "attack"
		}
		logger.LogTimeline(eval.TimelineEntry{
			WindowID:       w,
			TrueLabel:      trueLabel,
			PredictedLabel: predicted,
		})
	}

	summary, err := logger.Finish()
	if err != nil {
		log.Printf("experiment finish: %v", err)
		return
	}
	log.Printf("experiment output: dir=%s precision=%.4f recall=%.4f f1=%.4f",
		dir, summary.Precision, summary.Recall, summary.F1Score)
}
