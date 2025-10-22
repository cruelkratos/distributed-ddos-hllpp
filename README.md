# HyperGoLog++: An Optimized HLL++ Implementation for Scalable Cardinality Estimation

![Go](https://img.shields.io/badge/Go-00ADD8?logo=Go&logoColor=white&style=for-the-badge)
![Python](https://img.shields.io/badge/Python-3776AB?style=for-the-badge&logo=python&logoColor=white)


## Overview

This project provides a high-performance Go implementation of the HyperLogLog++ (HLL++) cardinality estimation algorithm, based on the Google research paper "HyperLogLog in Practice: Algorithmic Engineering of a State of The Art Cardinality Estimation Algorithm".

The primary goal is to build a core component for a distributed system capable of tasks like real-time DDoS attack detection by efficiently estimating the number of unique source IPs across a network.

The implementation focuses on accuracy, memory efficiency, and concurrency, incorporating key optimizations from the HLL++ paper.

## Based On

- **Paper**: Heule, S., Nunkesser, M., & Hall, A. (2013). HyperLogLog in Practice: Algorithmic Engineering of a State of The Art Cardinality Estimation Algorithm. [Link to Paper PDF if available, e.g., on arXiv or ACM, otherwise omit]
- **Algorithm**: HyperLogLog++ (HLL++)

## Key Features

- **Core HLL++ Logic**: Implements the fundamental HyperLogLog counting structure.
- **Sparse Representation**: Uses a memory-efficient sparse format (based on p' = 25) for low cardinalities, drastically reducing memory usage for small sets.
- **Automatic Dense Transition**: Seamlessly converts from the sparse representation to the standard dense format when memory usage dictates, ensuring optimal performance and memory across all cardinality ranges.
- **Empirical Bias Correction**: Incorporates pre-computed bias correction data (using k-NN interpolation with k=6) to significantly improve accuracy for small-to-medium cardinalities, closely matching the paper's methodology.
- **Thread Safety**: Designed for concurrent use with internal locking mechanisms (configurable).
- **Configurable Modes**: Allows running in thread-safe (concurrent) or non-locking (single) modes via command-line flags.
- **Algorithm Selection**: Supports switching between the original HLL estimation logic and the improved HLL++ logic (with bias correction) via the `-algorithm` flag for comparison.
- **64-bit Hashing**: Utilizes 64-bit hash functions (configurable: xxhash or SHA256-based) to handle extremely large cardinalities without issues from hash collisions inherent in 32-bit implementations.
- **Benchmarking Suite**: Includes tools to generate benchmark data and analyze performance and accuracy.

## Project Structure
```
HLL-BTP/
├── go.mod                 # Go module definition
├── go.sum                 # Go module checksums
├── main.go                # Main executable for running benchmarks
├── config.json            # Configuration (precision, hash algorithm)
├── benchmarks.txt         # Example benchmark output file
├── analyze_benchmarks.py  # Python script to plot benchmark results
│
├── bias/
│   └── biasdata/
│       ├── data.go        # Embeds bias data
│       └── *.json         # Generated bias correction data (e.g., bias_data_p14.json)
│
├── dataclasses/           # Helper data structures (Sum, ZeroCounter)
│   ├── sum.go
│   └── zerocounter.go
│
├── general/               # Utility functions (hashing helpers, config readers, math)
│   ├── helperfunctions.go
│   └── ...
│
├── generate_bias_data/    # Standalone tool to generate bias correction data
│   └── main.go
│
├── models/                # Shared data models (e.g., BiasDataPoint, Thresholds) - You might need this dir
│   └── models.go          # Contains HLLPlusPlusThresholds map etc.
│
├── types/
│   ├── hll/               # Core HLL/HLL++ implementation (dense, bias correction, wrapper)
│   │   ├── HLL.go
│   │   ├── bias_correction.go
│   │   └── hllpp.go       # Contains hllpp_set struct
│   │
│   ├── sparse/            # Sparse representation implementation
│   │   └── sparse.go
│   │
│   └── register/          # Dense register implementation (6-bit packing)
│       ├── registers.go
│       └── helper/
│           └── hashing.go # Hashing implementations
│
└── README.md              # This file
```



## Getting Started

### Prerequisites

- Go 1.21 or later (for `max` function, otherwise adjust code)
- Python 3.x (for plotting)
- `matplotlib` Python library (`pip install matplotlib`)

### 1. Generate Bias Correction Data (if needed)

The HLL++ algorithm requires pre-computed bias data for optimal accuracy.
```bash
# Navigate to the bias generation tool directory
cd generate_bias_data

# Run the generator (this may take several minutes)
# It will create/update bias/biasdata/bias_data_p<PRECISION>.json
go run .

# Navigate back to the project root
cd ..
```

*(The main benchmark script also includes a check to generate this if missing when running in HLL++ mode)*

### 2. Build the Benchmark Executable

From the project root directory:
```bash
go build -o HyperGoLogBenchmark .
```

*(On Windows, this might create `HyperGoLogBenchmark.exe`)*

### 3. Run Benchmarks

Execute the compiled benchmark program with various flags:

**Basic HLL++ run (Concurrent):**
```bash
./HyperGoLogBenchmark -outputFile=bench_hllpp_concurrent.txt
```

**Basic HLL run (Concurrent):**
```bash
./HyperGoLogBenchmark -algorithm=hll -outputFile=bench_hll_concurrent.txt
```

**HLL++ run (Single Mode, 1 Worker):**
```bash
./HyperGoLogBenchmark -mode=single -numWorkers=1 -outputFile=bench_hllpp_single.txt
```

**HLL++ run with 8 Workers (Concurrent):**
```bash
./HyperGoLogBenchmark -numWorkers=8 -outputFile=bench_hllpp_8workers.txt
```

**Shorter Test (50M IPs, log every 500k):**
```bash
./HyperGoLogBenchmark -maxIPs=50000000 -logInterval=500000 -outputFile=bench_short.txt
```

#### Available Flags:

- `-algorithm`: `hllpp` (default) or `hll`
- `-mode`: `concurrent` (default) or `single`
- `-numWorkers`: Number of concurrent insertion goroutines (default: number of CPU cores)
- `-maxIPs`: Total number of IPs to process (default: 200,000,000)
- `-logInterval`: How often to log results (default: 1,000,000)
- `-outputFile`: File to save benchmark results (default: `benchmarks.txt`)
- *(Optional)* `-useLargeRangeCorrection`: `true` or `false` (default: `false`) - Only relevant for original hll mode comparison.

### 4. Analyze Results

Use the Python script to generate plots from a benchmark output file:
```bash
python analyze_benchmarks.py <your_benchmark_output_file.txt>
```

*(If no filename is provided, it defaults to `benchmarks.txt`)*

The script will display the plots and save them as image files.

## Future Work

- **Implement gRPC/REST Server**: Wrap the HLL++ instance in a network service (`/insert`, `/estimate`, `/getSketch`).
- **Implement Merge() Method**: Add functionality to merge two HLL sketches (essential for distributed counting).
- **Distributed Architecture**: Design and implement worker nodes and an aggregator node using the server and merge capabilities.
- **DDoS Detection Logic**: Implement time-windowed counting and threshold-based alerting on the aggregator.
- **Advanced Sparse Compression**: Implement variable-length and difference encoding for the `sparse_list` as described in the paper for further memory optimization.

 [![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)