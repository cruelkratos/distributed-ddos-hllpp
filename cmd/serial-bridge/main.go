// serial-bridge reads IPs forwarded by an Arduino (FWD:<ip> lines over USB serial)
// and injects them into a local Go agent via gRPC InjectIPs. This allows the Arduino
// to act as a lightweight sensor feeding the Pi's full-precision HLL agent.
//
// Usage:
//
//	go run ./cmd/serial-bridge --serial /dev/ttyACM0 --agent localhost:50052
//	go run ./cmd/serial-bridge --serial /dev/ttyUSB0 --agent 192.168.1.100:50052 --batch 100
package main

import (
	pb "HLL-BTP/server"
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.bug.st/serial"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	serialPort := flag.String("serial", "/dev/ttyACM0", "Serial port for Arduino.")
	baudRate := flag.Int("baud", 115200, "Serial baud rate.")
	agentAddr := flag.String("agent", "localhost:50052", "Agent's InjectIP gRPC address.")
	batchSize := flag.Int("batch", 50, "Batch size for gRPC InjectIPs calls.")
	flag.Parse()

	// Connect to Go agent via gRPC.
	conn, err := grpc.NewClient(*agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("gRPC connect failed: %v", err)
	}
	defer conn.Close()
	client := pb.NewHllServiceClient(conn)
	log.Printf("Connected to agent at %s", *agentAddr)

	// Open serial port.
	mode := &serial.Mode{BaudRate: *baudRate}
	port, err := serial.Open(*serialPort, mode)
	if err != nil {
		log.Fatalf("Failed to open serial port %s: %v", *serialPort, err)
	}
	defer port.Close()
	log.Printf("Opened serial port %s @ %d baud", *serialPort, *baudRate)

	// Drain Arduino startup messages.
	time.Sleep(2 * time.Second)

	// Drain any startup messages from Arduino.
	time.Sleep(100 * time.Millisecond)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	scanner := bufio.NewScanner(port)
	batch := make([]string, 0, *batchSize)
	forwarded := uint64(0)
	lastReport := time.Now()

	log.Println("Bridge active. Forwarding Arduino FWD: lines → Go agent.")

	for {
		select {
		case <-stop:
			log.Printf("Shutting down. Total forwarded: %d IPs", forwarded)
			return
		default:
		}

		if scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if strings.HasPrefix(line, "FWD:") {
				ip := line[4:]
				batch = append(batch, ip)

				if len(batch) >= *batchSize {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, err := client.InjectIPs(ctx, &pb.InjectIPsBatchRequest{
						Ips:     batch,
						ByteLen: 64,
					})
					cancel()
					if err != nil {
						log.Printf("InjectIPs failed: %v", err)
					}
					forwarded += uint64(len(batch))
					batch = batch[:0]
				}
			} else if line == "READY" || strings.HasPrefix(line, "micro_hll") {
				log.Printf("Arduino: %s", line)
			}

			// Periodic status report.
			if time.Since(lastReport) > 10*time.Second {
				log.Printf("Forwarded %d IPs total", forwarded)
				lastReport = time.Now()
			}
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Serial read error: %v", err)
			break
		}
	}

	// Flush remaining batch.
	if len(batch) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := client.InjectIPs(ctx, &pb.InjectIPsBatchRequest{
			Ips:     batch,
			ByteLen: 64,
		})
		cancel()
		if err != nil {
			log.Printf("Final InjectIPs failed: %v", err)
		}
		forwarded += uint64(len(batch))
	}

	fmt.Printf("Bridge stopped. Total forwarded: %d IPs\n", forwarded)
}
