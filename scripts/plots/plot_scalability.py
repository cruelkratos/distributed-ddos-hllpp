#!/usr/bin/env python3
"""Plot scalability: memory and CPU vs number of nodes."""
import sys
import csv
import matplotlib.pyplot as plt
import numpy as np
from collections import defaultdict

def main():
    if len(sys.argv) < 2:
        print("Usage: plot_scalability.py <combined_scalability.csv> [output.pdf]")
        sys.exit(1)

    csv_path = sys.argv[1]
    out_path = sys.argv[2] if len(sys.argv) > 2 else "scalability.pdf"

    # Aggregate by node count: compute mean/max RSS and goroutines per node count.
    per_node = defaultdict(lambda: {"rss": [], "heap": [], "goroutines": []})

    with open(csv_path) as f:
        reader = csv.DictReader(f)
        for row in reader:
            n = int(row["nodes"])
            per_node[n]["rss"].append(int(row["rss_bytes"]))
            per_node[n]["heap"].append(int(row["heap_bytes"]))
            per_node[n]["goroutines"].append(int(row["cpu_goroutines"]))

    node_counts = sorted(per_node.keys())
    avg_rss = [np.mean(per_node[n]["rss"]) / 1024 for n in node_counts]
    max_rss = [np.max(per_node[n]["rss"]) / 1024 for n in node_counts]
    avg_heap = [np.mean(per_node[n]["heap"]) / 1024 for n in node_counts]
    avg_goroutines = [np.mean(per_node[n]["goroutines"]) for n in node_counts]

    fig, axes = plt.subplots(1, 3, figsize=(16, 5))

    # RSS scaling.
    axes[0].plot(node_counts, avg_rss, "o-", color="#2196F3", label="Avg RSS", linewidth=2)
    axes[0].plot(node_counts, max_rss, "s--", color="#F44336", label="Peak RSS", linewidth=1.5)
    axes[0].set_xlabel("Number of Nodes")
    axes[0].set_ylabel("RSS (KB)")
    axes[0].set_title("Memory Scaling")
    axes[0].legend()
    axes[0].grid(alpha=0.3)

    # Heap scaling.
    axes[1].plot(node_counts, avg_heap, "o-", color="#4CAF50", linewidth=2)
    axes[1].set_xlabel("Number of Nodes")
    axes[1].set_ylabel("Heap (KB)")
    axes[1].set_title("Heap Memory Scaling")
    axes[1].grid(alpha=0.3)

    # Goroutine scaling.
    axes[2].plot(node_counts, avg_goroutines, "o-", color="#FF9800", linewidth=2)
    axes[2].set_xlabel("Number of Nodes")
    axes[2].set_ylabel("Goroutines")
    axes[2].set_title("Goroutine Count Scaling")
    axes[2].grid(alpha=0.3)

    plt.suptitle("Scalability: Resource Usage vs Node Count", fontsize=14, fontweight="bold")
    plt.tight_layout()
    plt.savefig(out_path, dpi=300, bbox_inches="tight")
    print(f"Saved: {out_path}")

if __name__ == "__main__":
    main()
