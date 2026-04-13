#!/usr/bin/env python3
"""Plot detection evaluation results: precision, recall, F1, FPR per detector."""
import sys
import os
import json
import matplotlib.pyplot as plt
import numpy as np

def load_summaries(ablation_dir):
    """Load summary.json from each detector subdirectory."""
    detectors = []
    for name in sorted(os.listdir(ablation_dir)):
        summary_path = os.path.join(ablation_dir, name, "experiment", "summary.json")
        if os.path.isfile(summary_path):
            with open(summary_path) as f:
                data = json.load(f)
            data["detector"] = name
            detectors.append(data)
    return detectors

def main():
    if len(sys.argv) < 2:
        print("Usage: plot_detection.py <ablation_dir> [output.pdf]")
        sys.exit(1)

    ablation_dir = sys.argv[1]
    out_path = sys.argv[2] if len(sys.argv) > 2 else "detection_comparison.pdf"

    summaries = load_summaries(ablation_dir)
    if not summaries:
        print(f"No summaries found in {ablation_dir}")
        sys.exit(1)

    names = [s["detector"] for s in summaries]
    precision = [s.get("precision", 0) for s in summaries]
    recall = [s.get("recall", 0) for s in summaries]
    f1 = [s.get("f1_score", 0) for s in summaries]
    fpr = [s.get("false_positive_rate", 0) for s in summaries]

    x = np.arange(len(names))
    width = 0.2

    fig, ax = plt.subplots(figsize=(10, 5))
    ax.bar(x - 1.5*width, precision, width, label="Precision", color="#2196F3")
    ax.bar(x - 0.5*width, recall, width, label="Recall", color="#4CAF50")
    ax.bar(x + 0.5*width, f1, width, label="F1 Score", color="#FF9800")
    ax.bar(x + 1.5*width, fpr, width, label="FPR", color="#F44336")

    ax.set_xlabel("Detector")
    ax.set_ylabel("Score")
    ax.set_title("Detection Performance Comparison")
    ax.set_xticks(x)
    ax.set_xticklabels(names, rotation=45, ha="right")
    ax.legend()
    ax.set_ylim(0, 1.1)
    ax.grid(axis="y", alpha=0.3)
    plt.tight_layout()
    plt.savefig(out_path, dpi=300, bbox_inches="tight")
    print(f"Saved: {out_path}")

if __name__ == "__main__":
    main()
