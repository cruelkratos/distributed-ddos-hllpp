#!/usr/bin/env python3
"""Convert CICDDoS2019 dataset to standardized format for dataset-replay.
Reads the original CSV and outputs: timestamp, src_ip, label, attack_type, byte_len
"""
import sys
import os
import csv
import glob

ATTACK_TYPE_MAP = {
    "BENIGN": "normal",
    "DrDoS_DNS": "UDP_FLOOD",
    "DrDoS_LDAP": "UDP_FLOOD",
    "DrDoS_MSSQL": "UDP_FLOOD",
    "DrDoS_NTP": "UDP_FLOOD",
    "DrDoS_NetBIOS": "UDP_FLOOD",
    "DrDoS_SNMP": "UDP_FLOOD",
    "DrDoS_SSDP": "UDP_FLOOD",
    "DrDoS_UDP": "UDP_FLOOD",
    "Syn": "SYN_FLOOD",
    "UDP-lag": "UDP_FLOOD",
    "TFTP": "UDP_FLOOD",
    "WebDDoS": "HTTP_FLOOD",
    "Portmap": "SCAN_PROBE",
    "LDAP": "UDP_FLOOD",
    "MSSQL": "UDP_FLOOD",
    "NetBIOS": "UDP_FLOOD",
    "UDP": "UDP_FLOOD",
    "UDPLag": "UDP_FLOOD",
}

def convert_file(input_path, writer):
    """Convert a single CICDDoS2019 CSV file."""
    with open(input_path, encoding="utf-8", errors="replace") as f:
        reader = csv.DictReader(f)
        # Normalize header names (strip whitespace).
        reader.fieldnames = [h.strip() for h in reader.fieldnames]
        
        for row in reader:
            try:
                timestamp = row.get("Timestamp", row.get(" Timestamp", ""))
                src_ip = row.get("Source IP", row.get(" Source IP", ""))
                label_raw = row.get("Label", row.get(" Label", "")).strip()
                byte_len = row.get("Total Length of Fwd Packets",
                                   row.get(" Total Length of Fwd Packets", "0"))
                
                label = "normal" if label_raw == "BENIGN" else "attack"
                attack_type = ATTACK_TYPE_MAP.get(label_raw, "UNKNOWN")
                if label == "normal":
                    attack_type = ""
                
                writer.writerow([timestamp.strip(), src_ip.strip(), label,
                                attack_type, byte_len.strip()])
            except Exception:
                continue

def main():
    if len(sys.argv) < 2:
        print("Usage: convert_cicddos2019.py <input_dir> [output.csv]")
        sys.exit(1)
    
    input_dir = sys.argv[1]
    output_path = sys.argv[2] if len(sys.argv) > 2 else os.path.join(input_dir, "standardized.csv")
    
    csv_files = sorted(glob.glob(os.path.join(input_dir, "**/*.csv"), recursive=True))
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
    
    # Count output rows.
    with open(output_path) as f:
        lines = sum(1 for _ in f) - 1
    print(f"Done: {lines} rows written to {output_path}")

if __name__ == "__main__":
    main()
