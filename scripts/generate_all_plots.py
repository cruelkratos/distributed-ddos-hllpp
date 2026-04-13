#!/usr/bin/env python3
"""Generate all publication-quality plots from benchmark/experiment results."""

import csv
import os
import sys
import numpy as np
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
from collections import defaultdict

RESULTS_DIR = sys.argv[1] if len(sys.argv) > 1 else "results"
PLOTS_DIR = os.path.join(RESULTS_DIR, "plots")
os.makedirs(PLOTS_DIR, exist_ok=True)

# Publication style
plt.rcParams.update({
    'font.size': 11,
    'axes.labelsize': 12,
    'axes.titlesize': 13,
    'xtick.labelsize': 10,
    'ytick.labelsize': 10,
    'legend.fontsize': 9,
    'figure.figsize': (7, 4.5),
    'figure.dpi': 150,
    'savefig.dpi': 300,
    'axes.grid': True,
    'grid.alpha': 0.3,
})

# ============================================================
# PLOT 1: Live Demo Timeline (Defense State + Unique IPs)
# ============================================================
def plot_live_demo():
    defense_file = os.path.join(RESULTS_DIR, "live_demo", "defense_timeline.csv")
    prom_file = os.path.join(RESULTS_DIR, "live_demo", "prometheus_samples.csv")
    if not os.path.exists(defense_file):
        print("Skipping live demo plot — no data")
        return

    # Parse defense timeline
    times, activated, scores = [], [], []
    with open(defense_file) as f:
        reader = csv.DictReader(f)
        for row in reader:
            times.append(float(row['elapsed_sec']))
            activated.append(1 if row['activated'] == 'true' else 0)
            scores.append(float(row['global_score']))

    # Parse prometheus for unique IPs
    ip_times, ip_values = [], []
    if os.path.exists(prom_file):
        start_ts = None
        with open(prom_file) as f:
            reader = csv.DictReader(f)
            for row in reader:
                if row['metric'] == 'ddos_unique_ips_current_window':
                    ts = float(row['timestamp'])
                    if start_ts is None:
                        start_ts = ts
                    ip_times.append(ts - start_ts)
                    ip_values.append(float(row['value']))

    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(8, 6), sharex=True,
                                     gridspec_kw={'height_ratios': [2, 1]})

    # Attack phase shading (20-60s based on demo script)
    for ax in [ax1, ax2]:
        ax.axvspan(20, 60, alpha=0.15, color='red', label='Attack Phase')

    # Unique IPs
    if ip_times:
        ax1.plot(ip_times, ip_values, 'b-', linewidth=1.5, label='Unique IPs (HLL++)')
        ax1.set_ylabel('Unique IPs / Window')
        ax1.legend(loc='upper left')
        ax1.set_title('Live ESP32 Demo: DDoS Detection Timeline')

    # Defense state
    ax2.fill_between(times, activated, step='post', alpha=0.6, color='red', label='Defense Activated')
    ax2.set_ylabel('Defense State')
    ax2.set_xlabel('Time (seconds)')
    ax2.set_yticks([0, 1])
    ax2.set_yticklabels(['Normal', 'Under Attack'])
    ax2.legend(loc='upper left')

    plt.tight_layout()
    out = os.path.join(PLOTS_DIR, "fig1_live_demo_timeline.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 2: Detection Performance Comparison (Bar Chart)
# ============================================================
def plot_detection_comparison():
    data_file = os.path.join(RESULTS_DIR, "detection_eval", "all_results.txt")
    if not os.path.exists(data_file):
        print("Skipping detection comparison plot — no data")
        return

    scenarios = {}
    current_scenario = None
    with open(data_file) as f:
        for line in f:
            line = line.strip()
            if line.startswith("=== SCENARIO"):
                current_scenario = line.split(":")[0].replace("=== ", "").strip()
                scenarios[current_scenario] = {}
            elif current_scenario and line and not line.startswith("detector") and not line.startswith("---") and not line.startswith("2026"):
                parts = line.split()
                if len(parts) >= 9:
                    name = parts[0]
                    scenarios[current_scenario][name] = {
                        'recall': float(parts[1]),
                        'precision': float(parts[2]),
                        'f1': float(parts[3]),
                        'fpr': float(parts[4]),
                    }

    if not scenarios:
        print("Skipping detection comparison — no parsed data")
        return

    # Use Scenario 1 (Standard) for the main comparison
    sc1_key = [k for k in scenarios if "SCENARIO 1" in k]
    if not sc1_key:
        sc1_key = list(scenarios.keys())[:1]
    data = scenarios[sc1_key[0]] if sc1_key else {}

    if not data:
        return

    detectors = list(data.keys())
    metrics = ['recall', 'precision', 'f1']
    x = np.arange(len(detectors))
    width = 0.25
    colors = ['#2196F3', '#4CAF50', '#FF9800']

    fig, ax = plt.subplots(figsize=(8, 5))
    for i, metric in enumerate(metrics):
        values = [data[d][metric] for d in detectors]
        bars = ax.bar(x + i * width, values, width, label=metric.upper(),
                      color=colors[i], edgecolor='black', linewidth=0.5)
        for bar, val in zip(bars, values):
            if val > 0:
                ax.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.02,
                        f'{val:.2f}', ha='center', va='bottom', fontsize=8)

    ax.set_xlabel('Detector')
    ax.set_ylabel('Score')
    ax.set_title('Detection Performance: Standard Scenario (15K attack IPs)')
    ax.set_xticks(x + width)
    ax.set_xticklabels([d.title() for d in detectors], rotation=15)
    ax.legend()
    ax.set_ylim(0, 1.15)

    out = os.path.join(PLOTS_DIR, "fig2_detection_comparison.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 3: Multi-Scenario Heatmap (F1 scores across scenarios)
# ============================================================
def plot_scenario_heatmap():
    data_file = os.path.join(RESULTS_DIR, "detection_eval", "all_results.txt")
    if not os.path.exists(data_file):
        return

    scenarios = {}
    current_scenario = None
    with open(data_file) as f:
        for line in f:
            line = line.strip()
            if line.startswith("=== SCENARIO"):
                # Extract short name
                full = line.replace("===", "").strip()
                parts = full.split(":")
                current_scenario = parts[0].strip() if len(parts) > 0 else full
                scenarios[current_scenario] = {}
            elif current_scenario and line and not line.startswith("detector") and not line.startswith("---") and not line.startswith("2026"):
                parts = line.split()
                if len(parts) >= 9:
                    scenarios[current_scenario][parts[0]] = float(parts[3])  # F1

    if len(scenarios) < 2:
        print("Skipping heatmap — need multiple scenarios")
        return

    scenario_names = list(scenarios.keys())
    all_detectors = set()
    for s in scenarios.values():
        all_detectors.update(s.keys())
    detector_names = sorted(all_detectors)

    matrix = np.zeros((len(detector_names), len(scenario_names)))
    for j, s in enumerate(scenario_names):
        for i, d in enumerate(detector_names):
            matrix[i, j] = scenarios[s].get(d, 0)

    fig, ax = plt.subplots(figsize=(9, 5))
    im = ax.imshow(matrix, cmap='RdYlGn', aspect='auto', vmin=0, vmax=1)

    ax.set_xticks(range(len(scenario_names)))
    short_names = [s.replace("SCENARIO ", "S") for s in scenario_names]
    ax.set_xticklabels(short_names, rotation=30, ha='right', fontsize=9)
    ax.set_yticks(range(len(detector_names)))
    ax.set_yticklabels([d.title() for d in detector_names])

    for i in range(len(detector_names)):
        for j in range(len(scenario_names)):
            val = matrix[i, j]
            color = 'white' if val < 0.5 else 'black'
            ax.text(j, i, f'{val:.2f}', ha='center', va='center', color=color, fontsize=9)

    plt.colorbar(im, ax=ax, label='F1 Score')
    ax.set_title('F1 Score Heatmap: Detectors × Attack Scenarios')
    ax.set_xlabel('Scenario')
    ax.set_ylabel('Detector')

    out = os.path.join(PLOTS_DIR, "fig3_scenario_heatmap.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 4: Memory Scalability (RSS vs Nodes)
# ============================================================
def plot_scalability():
    scalability_dir = os.path.join(RESULTS_DIR, "scalability")
    if not os.path.exists(scalability_dir):
        print("Skipping scalability plot — no data")
        return

    nodes_list = []
    rss_avg = []
    heap_avg = []
    goroutines_avg = []

    for f in sorted(os.listdir(scalability_dir)):
        if not f.endswith('.csv'):
            continue
        n = int(f.replace('nodes_', '').replace('.csv', ''))
        rss_vals, heap_vals, gor_vals = [], [], []
        with open(os.path.join(scalability_dir, f)) as fh:
            reader = csv.DictReader(fh)
            for row in reader:
                rss_vals.append(int(row['rss_bytes']) / 1024)  # KB
                heap_vals.append(int(row['heap_bytes']) / 1024)
                gor_vals.append(int(row['cpu_goroutines']))
        nodes_list.append(n)
        rss_avg.append(np.mean(rss_vals))
        heap_avg.append(np.mean(heap_vals))
        goroutines_avg.append(np.mean(gor_vals))

    order = np.argsort(nodes_list)
    nodes_list = [nodes_list[i] for i in order]
    rss_avg = [rss_avg[i] for i in order]
    heap_avg = [heap_avg[i] for i in order]
    goroutines_avg = [goroutines_avg[i] for i in order]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4.5))

    ax1.plot(nodes_list, rss_avg, 'bo-', linewidth=2, markersize=6, label='RSS')
    ax1.plot(nodes_list, heap_avg, 'rs--', linewidth=2, markersize=6, label='Heap')
    ax1.set_xlabel('Number of Nodes')
    ax1.set_ylabel('Memory (KB)')
    ax1.set_title('Memory Scalability')
    ax1.legend()
    ax1.set_xscale('log')

    ax2.plot(nodes_list, goroutines_avg, 'g^-', linewidth=2, markersize=6)
    ax2.set_xlabel('Number of Nodes')
    ax2.set_ylabel('Goroutines')
    ax2.set_title('Goroutine Scalability')

    # Add per-node cost annotation
    if len(nodes_list) >= 2:
        cost_per_node = (rss_avg[-1] - rss_avg[0]) / (nodes_list[-1] - nodes_list[0])
        ax1.annotate(f'~{cost_per_node:.1f} KB/node', xy=(nodes_list[-1], rss_avg[-1]),
                     fontsize=9, ha='right')

    plt.tight_layout()
    out = os.path.join(PLOTS_DIR, "fig4_scalability.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 5: HLL++ Memory vs Exact Counting
# ============================================================
def plot_memory_comparison():
    mem_file = os.path.join(RESULTS_DIR, "memory_bench", "hll_vs_exact.csv")
    if not os.path.exists(mem_file):
        print("Skipping memory comparison — no data")
        return

    distinct, hll_mem, exact_mem, estimates = [], [], [], []
    with open(mem_file) as f:
        reader = csv.DictReader(f)
        for row in reader:
            distinct.append(int(row['distinct_ips']))
            hll_mem.append(int(row['hll_memory_bytes']) / 1024)
            exact_mem.append(int(row['exact_memory_bytes']) / 1024)
            estimates.append(int(row['hll_estimate']))

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4.5))

    # Memory comparison
    x = np.arange(len(distinct))
    width = 0.35
    ax1.bar(x - width/2, hll_mem, width, label='HLL++ (p=14)', color='#2196F3',
            edgecolor='black', linewidth=0.5)
    ax1.bar(x + width/2, exact_mem, width, label='Exact (HashMap)', color='#f44336',
            edgecolor='black', linewidth=0.5)
    ax1.set_xlabel('Distinct IPs')
    ax1.set_ylabel('Memory (KB)')
    ax1.set_title('Memory: HLL++ vs Exact Counting')
    ax1.set_xticks(x)
    ax1.set_xticklabels([f'{d:,}' for d in distinct], rotation=15)
    ax1.set_yscale('log')
    ax1.legend()

    # Savings ratio
    for i in range(len(distinct)):
        ratio = exact_mem[i] / hll_mem[i] if hll_mem[i] > 0 else 0
        ax1.text(i, max(exact_mem[i], hll_mem[i]) * 1.2, f'{ratio:.0f}×',
                 ha='center', fontsize=9, fontweight='bold')

    # Accuracy
    errors = [abs(e - d) / d * 100 for d, e in zip(distinct, estimates)]
    ax2.bar(range(len(distinct)), errors, color='#FF9800', edgecolor='black', linewidth=0.5)
    ax2.set_xlabel('Distinct IPs')
    ax2.set_ylabel('Relative Error (%)')
    ax2.set_title('HLL++ Estimation Accuracy (p=14)')
    ax2.set_xticks(range(len(distinct)))
    ax2.set_xticklabels([f'{d:,}' for d in distinct], rotation=15)
    for i, err in enumerate(errors):
        ax2.text(i, err + 0.1, f'{err:.2f}%', ha='center', fontsize=9)

    plt.tight_layout()
    out = os.path.join(PLOTS_DIR, "fig5_memory_comparison.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 6: ESP32 Attack Rate Sensitivity
# ============================================================
def plot_esp32_sensitivity():
    # Data from sensitivity tests
    rates = [500, 1000, 2000, 3000, 3500, 4000, 4500, 5000, 10000]
    detected = [False, False, False, True, False, True, True, True, True]

    fig, ax = plt.subplots(figsize=(7, 4))
    colors = ['#4CAF50' if not d else '#f44336' for d in detected]
    bars = ax.bar(range(len(rates)), [1 if d else 0 for d in detected],
                  color=colors, edgecolor='black', linewidth=0.5)

    ax.set_xticks(range(len(rates)))
    ax.set_xticklabels([f'{r:,}' for r in rates], rotation=30)
    ax.set_xlabel('Attack Rate (unique IPs/second)')
    ax.set_ylabel('Attack Detected')
    ax.set_yticks([0, 1])
    ax.set_yticklabels(['No', 'Yes'])
    ax.set_title('ESP32-C3 Edge Detection: Attack Rate Sensitivity')

    # Add threshold line
    ax.axvline(x=2.5, color='orange', linestyle='--', linewidth=2, label='Detection Threshold (~3000 IPs/s)')
    ax.legend()

    out = os.path.join(PLOTS_DIR, "fig6_esp32_sensitivity.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 7: HLL++ Precision vs Sketch Size Trade-off
# ============================================================
def plot_precision_tradeoff():
    precisions = [4, 6, 8, 10, 12, 14]
    sketch_bytes = [12, 48, 192, 768, 3072, 12288]
    std_errors = [26.0, 13.0, 6.5, 3.25, 1.62, 0.81]
    # Use 100k IP test accuracy
    accuracy_100k = [8.5, 2.9, 6.9, 2.9, 0.6, 0.1]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4.5))

    ax1.semilogy(precisions, sketch_bytes, 'bo-', linewidth=2, markersize=8)
    ax1.set_xlabel('Precision Parameter (p)')
    ax1.set_ylabel('Sketch Size (bytes)')
    ax1.set_title('Sketch Size vs Precision Parameter')
    for p, s in zip(precisions, sketch_bytes):
        ax1.annotate(f'{s}B', (p, s), textcoords="offset points",
                     xytext=(0, 10), ha='center', fontsize=9)

    # Highlight p=14 used in our system
    ax1.axvline(x=14, color='red', linestyle='--', alpha=0.5, label='Our System (p=14)')
    ax1.legend()

    ax2.plot(precisions, std_errors, 'rs-', linewidth=2, markersize=8, label='Theoretical StdErr')
    ax2.plot(precisions, accuracy_100k, 'g^--', linewidth=2, markersize=8, label='Actual Error (100K IPs)')
    ax2.set_xlabel('Precision Parameter (p)')
    ax2.set_ylabel('Error (%)')
    ax2.set_title('Estimation Error vs Precision Parameter')
    ax2.axvline(x=14, color='red', linestyle='--', alpha=0.5, label='Our System (p=14)')
    ax2.legend()

    plt.tight_layout()
    out = os.path.join(PLOTS_DIR, "fig7_precision_tradeoff.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 8: Ablation Study (grouped bars per detector)
# ============================================================
def plot_ablation():
    ablation_file = os.path.join(RESULTS_DIR, "ablation", "ablation.txt")
    if not os.path.exists(ablation_file):
        print("Skipping ablation plot — no data")
        return

    data = {}
    with open(ablation_file) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("detector") or line.startswith("---") or line.startswith("2026"):
                continue
            parts = line.split()
            if len(parts) >= 9:
                data[parts[0]] = {
                    'recall': float(parts[1]),
                    'precision': float(parts[2]),
                    'f1': float(parts[3]),
                    'fpr': float(parts[4]),
                }

    if not data:
        return

    detectors = list(data.keys())
    f1_scores = [data[d]['f1'] for d in detectors]
    recall_scores = [data[d]['recall'] for d in detectors]
    precision_scores = [data[d]['precision'] for d in detectors]

    fig, ax = plt.subplots(figsize=(8, 5))
    x = np.arange(len(detectors))
    width = 0.25

    ax.bar(x - width, recall_scores, width, label='Recall', color='#2196F3',
           edgecolor='black', linewidth=0.5)
    ax.bar(x, precision_scores, width, label='Precision', color='#4CAF50',
           edgecolor='black', linewidth=0.5)
    ax.bar(x + width, f1_scores, width, label='F1', color='#FF9800',
           edgecolor='black', linewidth=0.5)

    ax.set_xlabel('Detector Component')
    ax.set_ylabel('Score')
    ax.set_title('Ablation Study: Individual Detector Contribution')
    ax.set_xticks(x)
    ax.set_xticklabels([d.upper() for d in detectors], rotation=15)
    ax.set_ylim(0, 1.15)
    ax.legend()

    # Add F1 labels
    for i, f1 in enumerate(f1_scores):
        ax.text(i + width, f1 + 0.02, f'{f1:.2f}', ha='center', fontsize=8)

    out = os.path.join(PLOTS_DIR, "fig8_ablation.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# PLOT 9: System Architecture Memory Budget
# ============================================================
def plot_memory_budget():
    components = ['HLL++ (p=14)', 'LODA', 'HST', 'CMS\n(Rate Limiter)', 'State\nMachine', 'Other']
    sizes_kb = [24.0, 2.6, 1.4, 4.0, 0.1, 0.2]
    colors = ['#2196F3', '#4CAF50', '#FF9800', '#9C27B0', '#f44336', '#607D8B']

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(10, 4.5))

    # Pie chart
    ax1.pie(sizes_kb, labels=components, colors=colors, autopct='%1.1f%%',
            startangle=90, pctdistance=0.85)
    ax1.set_title(f'Memory Budget per Node\n(Total: {sum(sizes_kb):.1f} KB)')

    # Bar chart with absolute values
    bars = ax2.barh(components, sizes_kb, color=colors, edgecolor='black', linewidth=0.5)
    ax2.set_xlabel('Memory (KB)')
    ax2.set_title('Component Memory Usage')
    for bar, val in zip(bars, sizes_kb):
        ax2.text(bar.get_width() + 0.2, bar.get_y() + bar.get_height()/2,
                 f'{val:.1f} KB', va='center', fontsize=9)

    plt.tight_layout()
    out = os.path.join(PLOTS_DIR, "fig9_memory_budget.png")
    plt.savefig(out)
    plt.close()
    print(f"  Saved {out}")

# ============================================================
# Run all plots
# ============================================================
print("Generating publication-quality plots...")
print(f"Results dir: {RESULTS_DIR}")
print(f"Output dir: {PLOTS_DIR}")
print()

plot_live_demo()
plot_detection_comparison()
plot_scenario_heatmap()
plot_scalability()
plot_memory_comparison()
plot_esp32_sensitivity()
plot_precision_tradeoff()
plot_ablation()
plot_memory_budget()

print(f"\nAll plots saved to {PLOTS_DIR}/")
print("Files:")
for f in sorted(os.listdir(PLOTS_DIR)):
    if f.endswith('.png'):
        size = os.path.getsize(os.path.join(PLOTS_DIR, f))
        print(f"  {f} ({size/1024:.0f} KB)")
