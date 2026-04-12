// cmd/udp-flood/main.go — Sends random IPs via UDP for the ESP32-C3 agent.
// Usage:  go run ./cmd/udp-flood -target <esp32-ip>:50052
//
// Three phases: normal → attack → recovery.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"time"
)

func main() {
	target := flag.String("target", "10.201.115.200:50052", "ESP32 UDP address:port")
	normalRate := flag.Int("rate", 50, "IPs/s during normal phase")
	attackRate := flag.Int("attack-rate", 5000, "IPs/s during attack phase")
	normalDur := flag.Duration("normal", 60*time.Second, "Normal phase duration")
	attackDur := flag.Duration("attack", 30*time.Second, "Attack phase duration")
	recoveryDur := flag.Duration("recovery", 60*time.Second, "Recovery phase duration")
	flag.Parse()

	conn, err := net.Dial("udp", *target)
	if err != nil {
		fmt.Printf("dial %s: %v\n", *target, err)
		return
	}
	defer conn.Close()
	fmt.Printf("UDP target: %s\n\n", *target)

	sendPhase("NORMAL", conn, *normalRate, *normalDur)
	sendPhase("ATTACK", conn, *attackRate, *attackDur)
	sendPhase("RECOVERY", conn, *normalRate, *recoveryDur)
	fmt.Println("\nDone.")
}

func sendPhase(name string, conn net.Conn, ipsPerSec int, dur time.Duration) {
	fmt.Printf("[%s] %d IPs/s for %s\n", name, ipsPerSec, dur)

	batchSize := 50 // Max IPs per UDP packet (~900 bytes < MTU).
	if ipsPerSec < batchSize {
		batchSize = ipsPerSec
	}
	intervalNs := float64(time.Second) * float64(batchSize) / float64(ipsPerSec)
	ticker := time.NewTicker(time.Duration(intervalNs))
	defer ticker.Stop()

	deadline := time.Now().Add(dur)
	sent := 0
	for range ticker.C {
		if time.Now().After(deadline) {
			break
		}
		buf := make([]byte, 0, batchSize*18) // "255.255.255.255\n" ≈ 16 bytes
		for i := 0; i < batchSize; i++ {
			ip := fmt.Sprintf("%d.%d.%d.%d\n",
				rand.Intn(256), rand.Intn(256),
				rand.Intn(256), rand.Intn(256))
			buf = append(buf, ip...)
		}
		conn.Write(buf)
		sent += batchSize
	}
	fmt.Printf("[%s] sent %d IPs\n\n", name, sent)
}
