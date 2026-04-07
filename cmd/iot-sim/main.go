// IoT device traffic simulator. Connects to an agent's InjectIP gRPC endpoint
// and sends synthetic traffic at configurable rates.
package main

import (
	pb "HLL-BTP/server"
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	agentAddr := flag.String("agent", "localhost:50052", "Agent's InjectIP gRPC address.")
	profile := flag.String("profile", "normal", "Traffic profile: normal, attack, mixed")
	normalRate := flag.Int("normal-rate", 50, "IPs per second during normal traffic.")
	attackRate := flag.Int("attack-rate", 5000, "IPs per second during attack traffic.")
	normalPool := flag.Int("normal-pool", 1000, "Pool of unique IPs for normal traffic.")
	attackDelay := flag.Duration("attack-delay", 30*time.Second, "Delay before attack starts in mixed mode.")
	attackDuration := flag.Duration("attack-duration", 60*time.Second, "Duration of attack phase in mixed mode.")
	seed := flag.Int64("seed", 0, "Random seed (0 = time-based).")
	flag.Parse()

	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(*seed))

	// Pre-generate normal IP pool.
	normalIPs := make([]string, *normalPool)
	for i := range normalIPs {
		normalIPs[i] = fmt.Sprintf("10.%d.%d.%d", rng.Intn(256), rng.Intn(256), rng.Intn(254)+1)
	}

	conn, err := grpc.NewClient(*agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to agent at %s: %v", *agentAddr, err)
	}
	defer conn.Close()
	client := pb.NewHllServiceClient(conn)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("iot-sim started: agent=%s profile=%s normalRate=%d attackRate=%d", *agentAddr, *profile, *normalRate, *attackRate)

	switch *profile {
	case "normal":
		runTraffic(client, normalIPs, *normalRate, rng, stop)
	case "attack":
		runAttack(client, *attackRate, rng, stop)
	case "mixed":
		runMixed(client, normalIPs, *normalRate, *attackRate, rng, *attackDelay, *attackDuration, stop)
	default:
		log.Fatalf("unknown profile: %s", *profile)
	}

	log.Println("iot-sim stopped")
}

func runTraffic(client pb.HllServiceClient, pool []string, rate int, rng *rand.Rand, stop chan os.Signal) {
	interval := time.Second / time.Duration(rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ip := pool[rng.Intn(len(pool))]
			inject(client, ip, 64)
		}
	}
}

func runAttack(client pb.HllServiceClient, rate int, rng *rand.Rand, stop chan os.Signal) {
	flushInterval := 100 * time.Millisecond
	batchSize := rate / 10 // IPs per 100ms batch
	if batchSize < 1 {
		batchSize = 1
	}
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			injectBatch(client, batchSize, rng, 1500)
		}
	}
}

func runMixed(client pb.HllServiceClient, pool []string, normalRate, attackRate int, rng *rand.Rand, attackDelay, attackDuration time.Duration, stop chan os.Signal) {
	log.Printf("mixed mode: normal for %s, then attack for %s, then normal again", attackDelay, attackDuration)

	// Phase 1: Normal traffic.
	normalInterval := time.Second / time.Duration(normalRate)
	normalTicker := time.NewTicker(normalInterval)
	attackTimer := time.NewTimer(attackDelay)
	defer normalTicker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-attackTimer.C:
			goto ATTACK
		case <-normalTicker.C:
			ip := pool[rng.Intn(len(pool))]
			inject(client, ip, 64)
		}
	}

ATTACK:
	log.Println("[MIXED] Attack phase started")
	attackFlush := 100 * time.Millisecond
	batchSize := attackRate / 10
	if batchSize < 1 {
		batchSize = 1
	}
	attackTicker := time.NewTicker(attackFlush)
	endTimer := time.NewTimer(attackDuration)
	defer attackTicker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-endTimer.C:
			goto RECOVERY
		case <-attackTicker.C:
			injectBatch(client, batchSize, rng, 1500)
		}
	}

RECOVERY:
	log.Println("[MIXED] Recovery phase — back to normal traffic")
	normalTicker2 := time.NewTicker(normalInterval)
	defer normalTicker2.Stop()
	for {
		select {
		case <-stop:
			return
		case <-normalTicker2.C:
			ip := pool[rng.Intn(len(pool))]
			inject(client, ip, 64)
		}
	}
}

func randomIP(rng *rand.Rand) string {
	return fmt.Sprintf("%d.%d.%d.%d", rng.Intn(224)+1, rng.Intn(256), rng.Intn(256), rng.Intn(254)+1)
}

func inject(client pb.HllServiceClient, ip string, byteLen uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.InjectIP(ctx, &pb.InjectIPRequest{Ip: ip, ByteLen: byteLen})
	if err != nil {
		// silently ignore — transient errors expected under load
		_ = err
	}
}

// injectBatch sends n IPs in a single batch gRPC call.
func injectBatch(client pb.HllServiceClient, n int, rng *rand.Rand, byteLen uint64) {
	ips := make([]string, n)
	for i := range ips {
		ips[i] = randomIP(rng)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.InjectIPs(ctx, &pb.InjectIPsBatchRequest{Ips: ips, ByteLen: byteLen})
	if err != nil {
		_ = err
	}
}
