#!/usr/bin/env python3
"""Convert BoT-IoT dataset to standardized format for dataset-replay.
Reads the original CSV and outputs: timestamp, src_ip, label, attack_type, byte_len
"""
import sys
import os
import csv
import glob

ATTACK_TYPE_MAP = {
    "Normal": "",
    "DDoS": "UDP_FLOOD",
    "DoS": "UDP_FLOOD",
    "Reconnaissance": "SCAN_PROBE",
    "Theft": "UNKNOWN",
}

SUBCATEGORY_MAP = {
    "TCP": "SYN_FLOOD",
    "UDP": "UDP_FLOOD",
    "HTTP": "HTTP_FLOOD",
    "Service_Scan": "SCAN_PROBE",
    "OS_Fingerprint": "SCAN_PROBE",
}

def convert_file(input_path, writer):
    with open(input_path, encoding="utf-8", errors="replace") as f:
        reader = csv.DictReader(f)
        for row in reader:
            try:
                timestamp = row.get("stime", row.get("Stime", "0"))
                src_ip = row.get("saddr", row.get("srcip", ""))
                category = row.get("category", row.get("Category", "Normal")).strip()
                subcategory = row.get("subcategory", row.get("Subcategory", "")).strip()
                byte_len = row.get("sbytes", row.get("Sbytes", "0"))
                attack_val = row.get("attack", row.get("Attack", "0")).strip()
                
                label = "normal" if attack_val == "0" or category == "Normal" else "attack"
                
                # Use subcategory for more specific classification.
                if subcategory and subcategory in SUBCATEGORY_MAP:
                    attack_type = SUBCATEGORY_MAP[subcategory]
                else:
                    attack_type = ATTACK_TYPE_MAP.get(category, "UNKNOWN")
                
                if label == "normal":
                    attack_type = ""
                
                writer.writerow([timestamp.strip(), src_ip.strip(), label,
                                attack_type, byte_len.strip()])
            except Exception:
                continue

def main():
    if len(sys.argv) < 2:
        print("Usage: convert_bot_iot.py <input_dir> [output.csv]")
        sys.exit(1)
    
    input_dir = sys.argv[1]
    output_path = sys.argv[2] if len(sys.argv) > 2 else os.path.join(input_dir, "standardized.csv")
    
    csv_files = sorted(glob.glob(os.path.join(input_dir, "*.csv")))
    csv_files = [f for f in csv_files if "standardized" not in f]
    
    if not csv_files:
        print(f"No CSV files found in {input_dir}")
        sys.exit(1)
    
    print(f"Converting {len(csv_files)} files to {output_path}")
    
    with open(output_path, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["timestamp", "src_ip", "label", "attack_type", "byte_len"])
        for csv_file in csv_files:
            print(f"  Processing: {csv_file}")
            convert_file(csv_file, writer)
    
    with open(output_path) as f:
        lines = sum(1 for _ in f) - 1
    print(f"Done: {lines} rows written to {output_path}")

if __name__ == "__main__":
    main()
