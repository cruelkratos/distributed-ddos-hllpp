#!/usr/bin/env bash
# download_datasets.sh — Download standard DDoS benchmark datasets.
# Creates datasets/ directory with subdirectories for each dataset.
# NOTE: Some datasets require manual registration. This script provides
# download commands where possible and instructions otherwise.
set -euo pipefail

DATASETS_DIR="${1:-datasets}"
mkdir -p "$DATASETS_DIR"

echo "=== DDoS Dataset Downloader ==="
echo "Target directory: $DATASETS_DIR"
echo ""

# --- CICDDoS2019 ---
echo "--- CICDDoS2019 ---"
echo "The CICDDoS2019 dataset must be downloaded from:"
echo "  https://www.unb.ca/cic/datasets/ddos-2019.html"
echo "Download the CSV files and place them in: $DATASETS_DIR/cicddos2019/"
mkdir -p "$DATASETS_DIR/cicddos2019"
echo "  Expected files: 03-11/*.csv (training day), 01-12/*.csv (testing day)"
echo ""

# --- UNSW-NB15 ---
echo "--- UNSW-NB15 ---"
echo "The UNSW-NB15 dataset can be downloaded from:"
echo "  https://research.unsw.edu.au/projects/unsw-nb15-dataset"
mkdir -p "$DATASETS_DIR/unsw-nb15"
echo "Download CSV files to: $DATASETS_DIR/unsw-nb15/"
echo "  Expected files: UNSW-NB15_1.csv through UNSW-NB15_4.csv"
echo ""

# --- BoT-IoT ---
echo "--- BoT-IoT ---"
echo "The BoT-IoT dataset can be downloaded from:"
echo "  https://research.unsw.edu.au/projects/bot-iot-dataset"
mkdir -p "$DATASETS_DIR/bot-iot"
echo "Download CSV files to: $DATASETS_DIR/bot-iot/"
echo ""

# --- CAIDA ---
echo "--- CAIDA (DDoS 2007) ---"
echo "CAIDA datasets require a research agreement:"
echo "  https://www.caida.org/catalog/datasets/ddos-20070804_dataset/"
mkdir -p "$DATASETS_DIR/caida"
echo "Place pcap files in: $DATASETS_DIR/caida/"
echo ""

echo "=== After downloading, run the conversion scripts: ==="
echo "  python3 scripts/datasets/convert_cicddos2019.py $DATASETS_DIR/cicddos2019/"
echo "  python3 scripts/datasets/convert_unsw_nb15.py $DATASETS_DIR/unsw-nb15/"
echo "  python3 scripts/datasets/convert_bot_iot.py $DATASETS_DIR/bot-iot/"
echo ""
echo "Each converter produces a standardized CSV with columns:"
echo "  timestamp, src_ip, label, attack_type, byte_len"
