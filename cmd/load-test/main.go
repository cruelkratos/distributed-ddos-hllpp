// load-test sends synthetic HLL++ sketches to the aggregator in three phases:
// 1. Baseline — normal traffic volume to seed the detector's history
// 2. Spike    — high-volume burst that should trigger a DDoS alert
// 3. Cooldown — traffic returns to normal
//
// Usage inside Kubernetes:
//
//	kubectl apply -f k8s/load-test-job.yaml
//	kubectl logs -f job/ddos-load-test
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	pb "HLL-BTP/server"
	"HLL-BTP/types/hll"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "Aggregator gRPC address.")

	baselineRounds := flag.Int("baseline-rounds", 10, "Number of baseline (normal) rounds.")
	baselineIPs := flag.Int("baseline-ips", 500, "Unique IPs per baseline round.")

	spikeRounds := flag.Int("spike-rounds", 3, "Number of spike (attack) rounds.")
	spikeIPs := flag.Int("spike-ips", 20000, "Unique IPs per spike round.")

	cooldownRounds := flag.Int("cooldown-rounds", 5, "Number of cooldown rounds after spike.")
	cooldownIPs := flag.Int("cooldown-ips", 500, "Unique IPs per cooldown round.")

	interval := flag.Duration("interval", 10*time.Second, "Interval between rounds (should match aggregator window duration).")
	flag.Parse()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()
	client := pb.NewHllServiceClient(conn)

	type phase struct {
		name   string
		rounds int
		ips    int
	}
	phases := []phase{
		{"BASELINE", *baselineRounds, *baselineIPs},
		{"SPIKE", *spikeRounds, *spikeIPs},
		{"COOLDOWN", *cooldownRounds, *cooldownIPs},
	}

	round := 0
	total := *baselineRounds + *spikeRounds + *cooldownRounds

	for _, p := range phases {
		log.Printf("=== Phase: %s (%d rounds × %d IPs) ===", p.name, p.rounds, p.ips)
		for i := 0; i < p.rounds; i++ {
			round++
			if err := pushRound(client, p.ips, round, total); err != nil {
				log.Fatalf("round %d: %v", round, err)
			}
			if round < total {
				log.Printf("  waiting %s before next round...", *interval)
				time.Sleep(*interval)
			}
		}
	}
	log.Println("=== Load test complete ===")
}

// pushRound builds a synthetic sketch with numIPs unique addresses and pushes it.
func pushRound(client pb.HllServiceClient, numIPs, round, total int) error {
	s := hll.GetHLLPP(true)
	for i := 0; i < numIPs; i++ {
		_ = s.Insert(fmt.Sprintf("%d.%d.%d.%d",
			(i/16581375)%256,
			(i/65025)%256,
			(i/255)%256,
			i%255,
		))
	}
	sketch, err := s.ExportSketch()
	if err != nil {
		return fmt.Errorf("export sketch: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err = client.MergeSketch(ctx, &pb.MergeRequest{Sketch: sketch})
	cancel()
	if err != nil {
		return fmt.Errorf("MergeSketch: %w", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := client.GetEstimate(ctx2, &pb.GetEstimateRequest{})
	cancel2()
	if err != nil {
		log.Printf("  [round %d/%d] pushed %d IPs — GetEstimate failed: %v", round, total, numIPs, err)
	} else {
		log.Printf("  [round %d/%d] pushed %d IPs → aggregator estimate: %d",
			round, total, numIPs, resp.Estimate)
	}
	return nil
}
