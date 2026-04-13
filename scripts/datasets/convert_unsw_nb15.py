#!/usr/bin/env python3
"""Convert UNSW-NB15 dataset to standardized format for dataset-replay.
Reads the original CSV and outputs: timestamp, src_ip, label, attack_type, byte_len
"""
import sys
import os
import csv
import glob

ATTACK_TYPE_MAP = {
    "Normal": "",
    "Fuzzers": "SCAN_PROBE",
    "Analysis": "SCAN_PROBE",
    "Backdoors": "UNKNOWN",
    "DoS": "UDP_FLOOD",
    "Exploits": "UNKNOWN",
    "Generic": "UNKNOWN",
    "Reconnaissance": "SCAN_PROBE",
    "Shellcode": "UNKNOWN",
    "Worms": "UNKNOWN",
}

def convert_file(input_path, writer):
    with open(input_path, encoding="utf-8", errors="replace") as f:
        reader = csv.DictReader(f)
        for row in reader:
            try:
                # UNSW-NB15 uses stime (start time) as epoch.
                timestamp = row.get("stime", row.get("Stime", "0"))
                src_ip = row.get("srcip", row.get("Srcip", ""))
                label_raw = row.get("attack_cat", row.get("Attack_cat", "Normal")).strip()
                byte_len = row.get("sbytes", row.get("Sbytes", "0"))
                
                if not label_raw or label_raw == " ":
                    label_raw = "Normal"
                
                label = "normal" if row.get("label", row.get("Label", "0")).strip() == "0" else "attack"
                attack_type = ATTACK_TYPE_MAP.get(label_raw, "UNKNOWN")
                if label == "normal":
                    attack_type = ""
                
                writer.writerow([timestamp.strip(), src_ip.strip(), label,
                                attack_type, byte_len.strip()])
            except Exception:
                continue

def main():
    if len(sys.argv) < 2:
        print("Usage: convert_unsw_nb15.py <input_dir> [output.csv]")
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
