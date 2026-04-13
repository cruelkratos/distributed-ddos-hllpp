#!/usr/bin/env python3
"""ESP32 serial logger — captures Serial output from ESP32-C3 and logs
metrics (free_heap, ship_latency, loop_time, HLL count) to CSV.

Usage: python3 scripts/esp32_serial_logger.py /dev/ttyUSB0 [output.csv]
"""
import sys
import re
import csv
import time
import serial

# Regex patterns for metrics from Serial output.
SHIP_RE = re.compile(
    r'\[SHIP\] HTTP (\d+)\s+count=(\d+)\s+heap=(\d+)\s+latency=(\d+)ms'
)
DEFENSE_RE = re.compile(r'\[DEFENSE\] (.*)')

def main():
    if len(sys.argv) < 2:
        print("Usage: esp32_serial_logger.py <serial_port> [output.csv] [baud_rate]")
        print("  Example: esp32_serial_logger.py /dev/ttyUSB0 esp32_log.csv 115200")
        sys.exit(1)

    port = sys.argv[1]
    csv_path = sys.argv[2] if len(sys.argv) > 2 else "esp32_metrics.csv"
    baud = int(sys.argv[3]) if len(sys.argv) > 3 else 115200

    print(f"Opening {port} at {baud} baud, logging to {csv_path}")

    ser = serial.Serial(port, baud, timeout=1)
    csv_file = open(csv_path, "w", newline="")
    writer = csv.writer(csv_file)
    writer.writerow(["timestamp", "http_code", "hll_count", "free_heap",
                      "ship_latency_ms", "event"])

    try:
        while True:
            line = ser.readline().decode("utf-8", errors="replace").strip()
            if not line:
                continue

            ts = time.strftime("%Y-%m-%d %H:%M:%S")
            print(f"[{ts}] {line}")

            # Parse ship metrics.
            m = SHIP_RE.search(line)
            if m:
                writer.writerow([
                    ts,
                    m.group(1),  # HTTP code
                    m.group(2),  # HLL count
                    m.group(3),  # free heap
                    m.group(4),  # ship latency ms
                    "ship"
                ])
                csv_file.flush()
                continue

            # Parse defense events.
            m = DEFENSE_RE.search(line)
            if m:
                writer.writerow([ts, "", "", "", "", f"defense: {m.group(1)}"])
                csv_file.flush()
                continue

    except KeyboardInterrupt:
        print("\nStopping...")
    finally:
        ser.close()
        csv_file.close()
        print(f"Saved: {csv_path}")

if __name__ == "__main__":
    main()
