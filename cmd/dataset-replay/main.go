// cmd/dataset-replay/main.go — Replays standard DDoS datasets into the detection pipeline.
// Supports CSV datasets (CICDDoS2019, UNSW-NB15, BoT-IoT) and pcap files.
// Generates ground-truth CSV files for evaluation.
package main

import (
	pb "HLL-BTP/server"
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type datasetRow struct {
	Timestamp  time.Time
	SrcIP      string
	DstIP      string
	Protocol   string
	Label      string // "BENIGN" or attack type
	AttackType string
	ByteLen    uint64
}

type windowGT struct {
	windowID    int
	attackCount int
	normalCount int
	attackTypes map[string]int
}

func main() {
	// Input source.
	csvPath := flag.String("csv", "", "Path to dataset CSV file.")
	// Column mappings (0-based).
	srcIPCol := flag.Int("src-ip-col", 1, "Column index for source IP.")
	labelCol := flag.Int("label-col", -1, "Column index for label (BENIGN vs attack). -1 to skip.")
	timestampCol := flag.Int("timestamp-col", 0, "Column index for timestamp.")
	attackTypeCol := flag.Int("attack-type-col", -1, "Column index for attack type. -1 = same as label.")
	byteLenCol := flag.Int("byte-len-col", -1, "Column index for packet byte length. -1 = use default 64.")
	// Output mode.
	mode := flag.String("mode", "grpc", "Output mode: grpc, udp")
	agentAddr := flag.String("agent", "localhost:50052", "Agent gRPC address (for grpc mode).")
	udpAddr := flag.String("udp-target", "", "UDP target address:port (for udp mode).")
	// Timing.
	speedMultiplier := flag.Float64("speed", 1.0, "Speed multiplier (2.0 = 2× faster).")
	windowDur := flag.Duration("window", 10*time.Second, "Window duration for ground-truth bucketing.")
	// Ground truth output.
	gtPath := flag.String("ground-truth", "ground_truth.csv", "Path to write ground-truth CSV.")
	// Limits.
	maxRows := flag.Int("max-rows", 0, "Max rows to process (0 = all).")
	skipHeader := flag.Bool("skip-header", true, "Skip the first row (header).")
	// Timestamp format.
	tsFormat := flag.String("ts-format", "2006-01-02 15:04:05", "Timestamp parse format.")
	flag.Parse()

	if *csvPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: dataset-replay --csv <path> [options]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Open CSV.
	f, err := os.Open(*csvPath)
	if err != nil {
		log.Fatalf("open CSV: %v", err)
	}
	defer f.Close()

	reader := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	if *skipHeader {
		if _, err := reader.Read(); err != nil {
			log.Fatalf("read header: %v", err)
		}
	}

	// Set up output.
	var sendFn func(ip string, byteLen uint64)
	switch *mode {
	case "grpc":
		conn, err := grpc.NewClient(*agentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("grpc connect: %v", err)
		}
		defer conn.Close()
		client := pb.NewHllServiceClient(conn)
		batch := make([]string, 0, 100)
		var batchBytes uint64
		sendFn = func(ip string, byteLen uint64) {
			batch = append(batch, ip)
			batchBytes = byteLen
			if len(batch) >= 100 {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				client.InjectIPs(ctx, &pb.InjectIPsBatchRequest{Ips: batch, ByteLen: batchBytes})
				cancel()
				batch = batch[:0]
			}
		}
	case "udp":
		if *udpAddr == "" {
			log.Fatal("--udp-target required for udp mode")
		}
		conn, err := net.Dial("udp", *udpAddr)
		if err != nil {
			log.Fatalf("udp dial: %v", err)
		}
		defer conn.Close()
		buf := make([]byte, 0, 1400)
		count := 0
		sendFn = func(ip string, _ uint64) {
			buf = append(buf, ip...)
			buf = append(buf, '\n')
			count++
			if count >= 50 || len(buf) > 1300 {
				conn.Write(buf)
				buf = buf[:0]
				count = 0
			}
		}
	default:
		log.Fatalf("unknown mode: %s", *mode)
	}

	// Ground-truth tracking.
	var windows []windowGT

	var firstTS time.Time
	var prevTS time.Time
	rowsProcessed := 0
	currentWindow := 0
	currentGT := windowGT{windowID: 0, attackTypes: make(map[string]int)}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}
		if *maxRows > 0 && rowsProcessed >= *maxRows {
			break
		}

		row := parseRow(record, *srcIPCol, *labelCol, *timestampCol, *attackTypeCol, *byteLenCol, *tsFormat)
		if row.SrcIP == "" {
			continue
		}

		// Timing.
		if firstTS.IsZero() {
			firstTS = row.Timestamp
			prevTS = row.Timestamp
		}

		// Compute delay from previous row.
		if !row.Timestamp.IsZero() && !prevTS.IsZero() && row.Timestamp.After(prevTS) {
			delay := row.Timestamp.Sub(prevTS)
			if *speedMultiplier > 0 {
				delay = time.Duration(float64(delay) / *speedMultiplier)
			}
			if delay > 0 && delay < 10*time.Second {
				time.Sleep(delay)
			}
			prevTS = row.Timestamp
		}

		// Window assignment.
		if !row.Timestamp.IsZero() && !firstTS.IsZero() {
			elapsed := row.Timestamp.Sub(firstTS)
			win := int(elapsed / *windowDur)
			if win > currentWindow {
				windows = append(windows, currentGT)
				for i := currentWindow + 1; i < win; i++ {
					windows = append(windows, windowGT{windowID: i, attackTypes: make(map[string]int)})
				}
				currentWindow = win
				currentGT = windowGT{windowID: win, attackTypes: make(map[string]int)}
			}
		}

		// Track ground truth.
		isAttack := row.Label != "" && !strings.EqualFold(row.Label, "BENIGN") && !strings.EqualFold(row.Label, "normal") && !strings.EqualFold(row.Label, "0")
		if isAttack {
			currentGT.attackCount++
			at := row.AttackType
			if at == "" {
				at = row.Label
			}
			currentGT.attackTypes[at]++
		} else {
			currentGT.normalCount++
		}

		sendFn(row.SrcIP, row.ByteLen)
		rowsProcessed++

		if rowsProcessed%10000 == 0 {
			log.Printf("[REPLAY] %d rows processed, window=%d", rowsProcessed, currentWindow)
		}
	}

	// Flush last window.
	windows = append(windows, currentGT)

	log.Printf("[REPLAY] Done: %d rows, %d windows", rowsProcessed, len(windows))

	// Write ground-truth CSV.
	writeGroundTruth(*gtPath, windows, *windowDur)
}

func parseRow(record []string, srcIPCol, labelCol, tsCol, atCol, blCol int, tsFormat string) datasetRow {
	var row datasetRow
	if srcIPCol >= 0 && srcIPCol < len(record) {
		row.SrcIP = strings.TrimSpace(record[srcIPCol])
	}
	if labelCol >= 0 && labelCol < len(record) {
		row.Label = strings.TrimSpace(record[labelCol])
	}
	if tsCol >= 0 && tsCol < len(record) {
		ts := strings.TrimSpace(record[tsCol])
		if t, err := time.Parse(tsFormat, ts); err == nil {
			row.Timestamp = t
		}
		// Try epoch seconds fallback.
		if row.Timestamp.IsZero() {
			if epoch, err := strconv.ParseFloat(ts, 64); err == nil {
				sec := int64(epoch)
				nsec := int64((epoch - float64(sec)) * 1e9)
				row.Timestamp = time.Unix(sec, nsec)
			}
		}
	}
	if row.Timestamp.IsZero() {
		row.Timestamp = time.Now()
	}
	if atCol >= 0 && atCol < len(record) {
		row.AttackType = strings.TrimSpace(record[atCol])
	}
	if blCol >= 0 && blCol < len(record) {
		if v, err := strconv.ParseUint(strings.TrimSpace(record[blCol]), 10, 64); err == nil {
			row.ByteLen = v
		}
	}
	if row.ByteLen == 0 {
		row.ByteLen = 64
	}
	// Validate IP format loosely.
	if net.ParseIP(row.SrcIP) == nil {
		// Try generating a deterministic IP from the string (for non-IP source identifiers).
		if row.SrcIP != "" && !strings.Contains(row.SrcIP, ".") {
			row.SrcIP = hashToIP(row.SrcIP)
		}
	}
	return row
}

func hashToIP(s string) string {
	h := uint32(0)
	for _, c := range s {
		h = h*31 + uint32(c)
	}
	return fmt.Sprintf("%d.%d.%d.%d", (h>>24)&0xFF|1, (h>>16)&0xFF, (h>>8)&0xFF, h&0xFF|1)
}

func writeGroundTruth(path string, windows []windowGT, windowDur time.Duration) {
	f, err := os.Create(path)
	if err != nil {
		log.Printf("failed to write ground truth: %v", err)
		return
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"window_id", "is_attack", "attack_count", "normal_count", "primary_attack_type", "window_duration_s"})
	for _, win := range windows {
		isAttack := "0"
		if win.attackCount > win.normalCount/2 {
			isAttack = "1"
		}
		primaryType := ""
		maxCount := 0
		for t, c := range win.attackTypes {
			if c > maxCount {
				maxCount = c
				primaryType = t
			}
		}
		w.Write([]string{
			strconv.Itoa(win.windowID),
			isAttack,
			strconv.Itoa(win.attackCount),
			strconv.Itoa(win.normalCount),
			primaryType,
			fmt.Sprintf("%.0f", windowDur.Seconds()),
		})
	}
	log.Printf("[GT] Wrote ground truth: %s (%d windows)", path, len(windows))
}

func init() {
	_ = rand.Int
}
