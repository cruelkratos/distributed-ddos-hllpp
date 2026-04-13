#!/usr/bin/env python3
"""Plot ablation study: grouped bar chart of all metrics per detector."""
import sys
import os
import json
import matplotlib.pyplot as plt
import numpy as np

def main():
    if len(sys.argv) < 2:
        print("Usage: plot_ablation.py <ablation_dir> [output.pdf]")
        sys.exit(1)

    ablation_dir = sys.argv[1]
    out_path = sys.argv[2] if len(sys.argv) > 2 else "ablation_study.pdf"

    summaries = []
    for name in sorted(os.listdir(ablation_dir)):
        summary_path = os.path.join(ablation_dir, name, "experiment", "summary.json")
        if os.path.isfile(summary_path):
            with open(summary_path) as f:
                data = json.load(f)
            data["detector"] = name
            summaries.append(data)

    if not summaries:
        print(f"No summaries found in {ablation_dir}")
        sys.exit(1)

    names = [s["detector"] for s in summaries]
    metrics_map = {
        "Precision": [s.get("precision", 0) for s in summaries],
        "Recall": [s.get("recall", 0) for s in summaries],
        "F1 Score": [s.get("f1_score", 0) for s in summaries],
        "FPR": [s.get("false_positive_rate", 0) for s in summaries],
        "FNR": [s.get("false_negative_rate", 0) for s in summaries],
    }

    fig, axes = plt.subplots(2, 1, figsize=(12, 10))

    # Detection metrics comparison.
    x = np.arange(len(names))
    width = 0.15
    colors = ["#2196F3", "#4CAF50", "#FF9800", "#F44336", "#9C27B0"]

    for i, (metric, values) in enumerate(metrics_map.items()):
        offset = (i - len(metrics_map) / 2 + 0.5) * width
        axes[0].bar(x + offset, values, width, label=metric, color=colors[i])

    axes[0].set_xlabel("Detector")
    axes[0].set_ylabel("Score")
    axes[0].set_title("Ablation Study: Detection Metrics per Detector")
    axes[0].set_xticks(x)
    axes[0].set_xticklabels(names, rotation=45, ha="right")
    axes[0].legend(loc="upper right")
    axes[0].set_ylim(0, 1.15)
    axes[0].grid(axis="y", alpha=0.3)

    # Resource metrics comparison.
    avg_rss = [s.get("avg_rss_bytes", 0) / 1024 for s in summaries]
    peak_rss = [s.get("peak_rss_bytes", 0) / 1024 for s in summaries]

    axes[1].bar(x - 0.2, avg_rss, 0.35, label="Avg RSS (KB)", color="#2196F3")
    axes[1].bar(x + 0.2, peak_rss, 0.35, label="Peak RSS (KB)", color="#F44336")
    axes[1].set_xlabel("Detector")
    axes[1].set_ylabel("Memory (KB)")
    axes[1].set_title("Ablation Study: Memory Usage per Detector")
    axes[1].set_xticks(x)
    axes[1].set_xticklabels(names, rotation=45, ha="right")
    axes[1].legend()
    axes[1].grid(axis="y", alpha=0.3)

    plt.tight_layout()
    plt.savefig(out_path, dpi=300, bbox_inches="tight")
    print(f"Saved: {out_path}")

if __name__ == "__main__":
    main()
