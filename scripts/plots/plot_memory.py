#!/usr/bin/env python3
"""Plot memory usage over time from benchmark CSV."""
import sys
import csv
import matplotlib.pyplot as plt

def main():
    if len(sys.argv) < 2:
        print("Usage: plot_memory.py <benchmark.csv> [output.pdf]")
        sys.exit(1)

    csv_path = sys.argv[1]
    out_path = sys.argv[2] if len(sys.argv) > 2 else "memory_usage.pdf"

    timestamps, rss, heap, stack, phases = [], [], [], [], []
    with open(csv_path) as f:
        reader = csv.DictReader(f)
        for row in reader:
            timestamps.append(int(row["timestamp"]))
            rss.append(int(row["rss_bytes"]) / 1024)
            heap.append(int(row["heap_bytes"]) / 1024)
            stack.append(int(row["stack_bytes"]) / 1024)
            phases.append(row["phase"])

    if not timestamps:
        print("No data found")
        sys.exit(1)

    # Normalize timestamps to relative seconds.
    t0 = timestamps[0]
    t = [(ts - t0) for ts in timestamps]

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(12, 8), sharex=True)

    # RSS plot with phase shading.
    ax1.plot(t, rss, label="RSS", color="#2196F3", linewidth=2)
    ax1.set_ylabel("Memory (KB)")
    ax1.set_title("RSS Memory Over Time")
    ax1.legend()
    ax1.grid(alpha=0.3)

    # Shade attack regions.
    in_attack = False
    attack_start = 0
    for i, phase in enumerate(phases):
        if phase == "attack" and not in_attack:
            attack_start = t[i]
            in_attack = True
        elif phase != "attack" and in_attack:
            ax1.axvspan(attack_start, t[i], alpha=0.15, color="red", label="Attack" if attack_start == t[0] else "")
            ax2.axvspan(attack_start, t[i], alpha=0.15, color="red")
            in_attack = False
    if in_attack:
        ax1.axvspan(attack_start, t[-1], alpha=0.15, color="red")
        ax2.axvspan(attack_start, t[-1], alpha=0.15, color="red")

    # Heap + Stack breakdown.
    ax2.fill_between(t, 0, stack, alpha=0.6, label="Stack", color="#FF9800")
    ax2.fill_between(t, stack, [s + h for s, h in zip(stack, heap)], alpha=0.6, label="Heap", color="#4CAF50")
    ax2.set_xlabel("Time (s)")
    ax2.set_ylabel("Memory (KB)")
    ax2.set_title("Heap vs Stack Breakdown")
    ax2.legend()
    ax2.grid(alpha=0.3)

    plt.tight_layout()
    plt.savefig(out_path, dpi=300, bbox_inches="tight")
    print(f"Saved: {out_path}")

if __name__ == "__main__":
    main()
