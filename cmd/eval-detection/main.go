// eval-detection runs the DDoS detection evaluation scenario and outputs metrics for the report.
package main

import (
	"HLL-BTP/ddos/detector"
	"HLL-BTP/ddos/eval"
	"flag"
	"fmt"
	"log"
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
	detectorName := flag.String("detector", "threshold", "Detector to use: threshold | zscore | ewma | compare")
	// Z-score flags
	zsHistory := flag.Int("zs-history", 20, "Z-score: rolling history length.")
	zsThreshold := flag.Float64("zs-threshold", 3.0, "Z-score: sigma threshold to trigger alert.")
	// EWMA flags
	ewmaAlpha := flag.Float64("ewma-alpha", 0.2, "EWMA: smoothing factor (0<α≤1).")
	ewmaDeviation := flag.Float64("ewma-deviation", 2.0, "EWMA: proportional deviation above baseline (e.g. 2.0 = 200%).")
	ewmaWarmup := flag.Int("ewma-warmup", 5, "EWMA: warmup windows before detection starts.")
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
		default: // "threshold"
			return nil // eval.Run uses ThresholdDetector when Detector is nil
		}
	}

	if *detectorName == "compare" {
		names := []string{"threshold", "zscore", "ewma", "ensemble"}
		fmt.Printf("%-12s  %8s  %4s  %4s  %4s  %4s  %10s\n",
			"detector", "recall", "TP", "FP", "FN", "TN", "ttd")
		fmt.Println("--------------------------------------------------------------")
		for _, name := range names {
			sc := baseScenario
			sc.Detector = buildDetector(name)
			res, err := eval.Run(sc)
			if err != nil {
				log.Fatalf("eval(%s): %v", name, err)
			}
			fmt.Printf("%-12s  %8.4f  %4d  %4d  %4d  %4d  %10s\n",
				name, res.Recall, res.TruePositives, res.FalsePositives,
				res.FalseNegatives, res.TrueNegatives, res.TimeToDetect)
		}
		return
	}

	scenario := baseScenario
	scenario.Detector = buildDetector(*detectorName)

	log.Printf("Running scenario: detector=%s window=%s threshold=%d normal=%d attack=%d windows=%d attack_windows=%v",
		*detectorName, *windowDur, *threshold, *normalIPs, *attackIPs, *totalWindows, attackList)

	res, err := eval.Run(scenario)
	if err != nil {
		log.Fatalf("eval: %v", err)
	}

	fmt.Println("--- Detection evaluation results ---")
	fmt.Printf("Detector:          %s\n", *detectorName)
	fmt.Printf("Recall:            %.4f\n", res.Recall)
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
}
