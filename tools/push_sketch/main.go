// push_sketch is a development/testing tool that creates a synthetic HLL++ sketch
// and pushes it to a running aggregator via gRPC MergeSketch.
// Use it to test the aggregator pipeline without needing root/pcap.
//
// Usage:
//
//	go run ./tools/push_sketch [flags]
//
// Flags:
//
//	-addr      aggregator gRPC address (default: localhost:50051)
//	-ips       number of synthetic unique IPs to insert (default: 2000)
//	-rounds    number of times to push (default: 1)
//	-interval  wait between rounds (default: 1s)
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
	numIPs := flag.Int("ips", 2000, "Number of unique IPs to insert into the sketch.")
	rounds := flag.Int("rounds", 1, "Number of push rounds.")
	interval := flag.Duration("interval", time.Second, "Wait between rounds.")
	flag.Parse()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer conn.Close()
	client := pb.NewHllServiceClient(conn)

	for round := 1; round <= *rounds; round++ {
		s := hll.GetHLLPP(true)
		for i := 0; i < *numIPs; i++ {
			// Generate IPs spread across /8 space for realism
			_ = s.Insert(fmt.Sprintf("%d.%d.%d.%d", (i/16581375)%256, (i/65025)%256, (i/255)%256, i%255))
		}
		sketch, err := s.ExportSketch()
		if err != nil {
			log.Fatalf("export sketch: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = client.MergeSketch(ctx, &pb.MergeRequest{Sketch: sketch})
		cancel()
		if err != nil {
			log.Fatalf("MergeSketch round %d: %v", round, err)
		}

		// Also get the current estimate from the aggregator
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := client.GetEstimate(ctx2, &pb.GetEstimateRequest{})
		cancel2()
		if err != nil {
			log.Printf("GetEstimate round %d: %v", round, err)
		} else {
			log.Printf("[round %d/%d] pushed %d IPs → aggregator estimate: %d", round, *rounds, *numIPs, resp.Estimate)
		}

		if round < *rounds {
			time.Sleep(*interval)
		}
	}
}
