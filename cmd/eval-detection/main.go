// eval-detection runs the DDoS detection evaluation scenario and outputs metrics for the report.
package main

import (
	"HLL-BTP/ddos/eval"
	"flag"
	"fmt"
	"log"
	"time"
)

func main() {
	windowDur := flag.Duration("window", 2*time.Second, "Window duration.")
	threshold := flag.Uint64("threshold", 5000, "Threshold for attack.")
	normalIPs := flag.Int("normal", 300, "Distinct IPs per window during normal.")
	attackIPs := flag.Int("attack", 15000, "Distinct IPs per window during attack.")
	totalWindows := flag.Int("windows", 12, "Total number of windows.")
	attackFrom := flag.Int("attack-from", 4, "First attack window index (0-based).")
	attackCount := flag.Int("attack-count", 4, "Number of consecutive attack windows.")
	seed := flag.Int64("seed", 42, "RNG seed for reproducibility.")
	outCSV := flag.String("out", "", "Optional: write results CSV to this path.")
	flag.Parse()

	attackList := make([]int, 0, *attackCount)
	for i := 0; i < *attackCount; i++ {
		attackList = append(attackList, *attackFrom+i)
	}

	scenario := eval.Scenario{
		WindowDuration:   *windowDur,
		CheckInterval:   time.Second,
		Threshold:       *threshold,
		NormalPerWindow: *normalIPs,
		AttackPerWindow: *attackIPs,
		TotalWindows:    *totalWindows,
		AttackWindowList: attackList,
		Seed:            *seed,
	}

	log.Printf("Running scenario: window=%s threshold=%d normal=%d attack=%d windows=%d attack_windows=%v",
		*windowDur, *threshold, *normalIPs, *attackIPs, *totalWindows, attackList)

	res, err := eval.Run(scenario)
	if err != nil {
		log.Fatalf("eval: %v", err)
	}

	fmt.Println("--- Detection evaluation results ---")
	fmt.Printf("Recall:           %.4f\n", res.Recall)
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
