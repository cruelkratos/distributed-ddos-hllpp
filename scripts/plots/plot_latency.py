#!/usr/bin/env python3
"""Plot detection latency CDF and timeline from experiment data."""
import sys
import os
import csv
import json
import matplotlib.pyplot as plt
import numpy as np

def main():
    if len(sys.argv) < 2:
        print("Usage: plot_latency.py <experiment_dir> [output.pdf]")
        sys.exit(1)

    exp_dir = sys.argv[1]
    out_path = sys.argv[2] if len(sys.argv) > 2 else "detection_latency.pdf"

    timeline_path = os.path.join(exp_dir, "timeline.csv")
    if not os.path.isfile(timeline_path):
        print(f"No timeline.csv in {exp_dir}")
        sys.exit(1)

    latencies = []
    scores = []
    window_ids = []
    labels = []

    with open(timeline_path) as f:
        reader = csv.DictReader(f)
        for row in reader:
            window_ids.append(int(row["window_id"]))
            scores.append(float(row["score"]))
            labels.append(row["true_label"])
            lat = float(row["latency_ms"])
            if lat > 0:
                latencies.append(lat)

    fig, axes = plt.subplots(1, 2, figsize=(14, 5))

    # CDF of detection latencies.
    if latencies:
        sorted_lat = np.sort(latencies)
        cdf = np.arange(1, len(sorted_lat) + 1) / len(sorted_lat)
        axes[0].plot(sorted_lat, cdf, linewidth=2, color="#2196F3")
        axes[0].axhline(y=0.5, color="gray", linestyle="--", alpha=0.5, label="p50")
        axes[0].axhline(y=0.95, color="gray", linestyle=":", alpha=0.5, label="p95")
        axes[0].set_xlabel("Detection Latency (ms)")
        axes[0].set_ylabel("CDF")
        axes[0].set_title("Detection Latency CDF")
        axes[0].legend()
        axes[0].grid(alpha=0.3)
    else:
        axes[0].text(0.5, 0.5, "No latency data", ha="center", va="center", transform=axes[0].transAxes)
        axes[0].set_title("Detection Latency CDF")

    # Score timeline with ground truth shading.
    colors = ["green" if l == "normal" else "red" for l in labels]
    axes[1].bar(window_ids, scores, color=colors, alpha=0.7, width=0.8)
    axes[1].set_xlabel("Window ID")
    axes[1].set_ylabel("Score")
    axes[1].set_title("Anomaly Score per Window")
    axes[1].grid(axis="y", alpha=0.3)

    # Add legend.
    from matplotlib.patches import Patch
    legend_elements = [Patch(facecolor="green", alpha=0.7, label="Normal"),
                       Patch(facecolor="red", alpha=0.7, label="Attack")]
    axes[1].legend(handles=legend_elements)

    plt.tight_layout()
    plt.savefig(out_path, dpi=300, bbox_inches="tight")
    print(f"Saved: {out_path}")

if __name__ == "__main__":
    main()
