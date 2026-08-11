# VFS Driver Benchmark Report

## Overview

This report compares the performance of the two VFS backend drivers: **BadgerDB** (Key-Value Embeddable DB) and **LocalStorage** (Native OS Filesystem).
Specifically, we focused on `Seek` performance, which is critical for video streaming (Range Requests) and random access.

## Environment Service

- **OS**: macOS (Darwin ARM64)
- **CPU**: Apple M4 Pro
- **Go Version**: 1.25.6
- **Test Date**: 2026-03-03

## Methodology

- **Scenario**: Creating a 50MB file and performing random `Seek` + `1KB Read` operations repeatedly.
- **Metric**: Throughput (MB/s) and Latency (ns/op).

## Results

### 1. Seek Performance (Random Access)

| Driver | Operations (N) | Latency (ns/op) | Throughput (MB/s) | Allocations (B/op) | Allocations (count/op) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **LocalStorage** | 1,464,468 | **818.1 ns** | **1,251.62 MB/s** | 1,024 B | 1 |
| **Badger (KV)** | 95,582 | **10,520 ns** | **97.34 MB/s** | 311,217 B | 5 |

> **Note**: Seek performance is measured by randomly hopping through a 50MB file and reading small chunks.

### 2. Write Performance (Create)

| Metric | LocalStorage (1KB) | Badger (1KB) | LocalStorage (4KB) | Badger (4KB) | LocalStorage (1MB) | Badger (1MB) |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Latency** | 176,632 ns | **132,492 ns** | 185,923 ns | **226,389 ns** | **414,520 ns** | 3,115,229 ns |
| **Throughput** | 5.80 MB/s | **7.73 MB/s** | 22.03 MB/s | **18.09 MB/s** | **2,529.61 MB/s** | 336.60 MB/s |

> **Observation**: **BadgerDB** is faster for small writes (1KB) likely due to LSM Tree buffering (MemTable). However, for large files (1MB), **LocalStorage** significantly outperforms Badger, likely due to direct sequential I/O efficiency versus Badger's chunking and LSM overhead.

### 3. Read Performance

| Metric | LocalStorage (1KB) | Badger (1KB) | LocalStorage (4KB) | Badger (4KB) | LocalStorage (1MB) | Badger (1MB) |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Latency** | 52,634 ns | **28,102 ns** | **52,498 ns** | 63,132 ns | **135,114 ns** | 511,624 ns |
| **Throughput** | 19.45 MB/s | **36.44 MB/s** | **78.02 MB/s** | 64.88 MB/s | **7,760.65 MB/s** | 2,049.56 MB/s |

> **Observation**: **LocalStorage** outperforms **BadgerDB** in read performance for both small and large files. While Badger is competitive for small reads, LocalStorage leverages OS filesystem caching effectively, especially for large sequential reads.

## Analysis

### LocalStorage (Native)

- **Strengths**: Superior raw throughput for large files and very low latency for Seek operations. Efficient use of OS filesystem features.
- **Best For**: Video streaming, large file storage, and high-throughput I/O workloads.

### BadgerDB (KV Store)

- **Strengths**: Good performance for small writes due to LSM tree architecture. Consistent performance across different file sizes relative to its architecture.
- **Weaknesses**: Significant overhead for `Seek` operations and large file I/O compared to native filesystem.
- **Best For**: Scenarios requiring a self-contained, portable filesystem (e.g., embedded databases, single-file distribution) where raw I/O throughput is secondary to portability or specific feature requirements.

## Conclusion

- **General Use**: **LocalStorage** is the recommended driver for general-purpose VFS needs, especially for media handling and large datasets.
- **Specialized Use**: Use **BadgerDB** when portability and single-file database characteristics are required, accepting the trade-off in raw I/O performance.
