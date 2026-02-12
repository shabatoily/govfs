# VFS Driver Benchmark Report

## Overview

This report compares the performance of the two VFS backend drivers: **BadgerDB** (Key-Value Embeddable DB) and **LocalStorage** (Native OS Filesystem).
Specifically, we focused on `Seek` performance, which is critical for video streaming (Range Requests) and random access.

## Environment Service

- **OS**: macOS (Darwin ARM64)
- **CPU**: Apple M4 Pro
- **Go Version**: 1.25.6
- **Test Date**: 2026-02-12

## Methodology

- **Scenario**: Creating a 50MB file and performing random `Seek` + `1KB Read` operations repeatedly.
- **Metric**: Throughput (MB/s) and Latency (ns/op).

## Results

### 1. Seek Performance (Random Access)

| Driver | Operations (N) | Latency (ns/op) | Throughput (MB/s) | Allocations (B/op) |
| :--- | :--- | :--- | :--- | :--- |
| **LocalStorage** | 476,498 | **2,171 ns** | **471.66 MB/s** | 1,024 B |
| **Badger (KV)** | 48,679 | **24,587 ns** | **41.65 MB/s** | 313,919 B |

> **Note**: Seek performance is measured by randomly hopping through a 50MB file and reading small chunks.

### 2. Write Performance (Create)

| Metric | LocalStorage (1KB) | Badger (1KB) | LocalStorage (1MB) | Badger (1MB) |
| :--- | :--- | :--- | :--- | :--- |
| **Latency** | 216,086 ns | **126,017 ns** | **726,594 ns** | 2,528,132 ns |
| **Throughput** | 4.74 MB/s | **8.13 MB/s** | **1,443.14 MB/s** | 414.76 MB/s |

> **Observation**: **BadgerDB** is faster for small writes (1KB) likely due to LSM Tree buffering (MemTable). However, for large files (1MB), **LocalStorage** significantly outperforms Badger, likely due to direct sequential I/O efficiency versus Badger's chunking and LSM overhead.

### 3. Read Performance

| Metric | LocalStorage (1KB) | Badger (1KB) | LocalStorage (1MB) | Badger (1MB) |
| :--- | :--- | :--- | :--- | :--- |
| **Latency** | **22,866 ns** | 24,553 ns | **170,583 ns** | 268,399 ns |
| **Throughput** | **44.78 MB/s** | 41.71 MB/s | **6,147.03 MB/s** | 3,906.79 MB/s |

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
