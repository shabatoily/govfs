# VFS Driver Benchmark Report

## Overview
This report compares the performance of the two VFS backend drivers: **BadgerDB** (Key-Value Embeddable DB) and **LocalStorage** (Native OS Filesystem).
Specifically, we focused on `Seek` performance, which is critical for video streaming (Range Requests) and random access.

## Environment Service
- **OS**: macOS (Darwin ARM64)
- **CPU**: Apple M4 Pro
- **Go Version**: 1.25.6
- **Test Date**: 2026-01-27

## Methodology
- **Scenario**: Creating a 50MB file and performing random `Seek` + `1KB Read` operations repeatedly.
- **Metric**: Throughput (MB/s) and Latency (ns/op).

## Results

### 1. Seek Performance (Random Access)

| Driver | Operations (N) | Latency (ns/op) | Throughput (MB/s) | Allocations (B/op) |
| :--- | :--- | :--- | :--- | :--- |
| **LocalStorage** | 1,660,374 | **729.3 ns** | **1,403.99 MB/s** | 1,024 B |
| **Badger (KV)** | 106,974 | **10,550 ns** | **97.06 MB/s** | 310,918 B |

> **Note**: Seek performance is measured by randomly hopping through a 50MB file and reading small chunks.

### 2. Write Performance (Create)

| Metric | LocalStorage (1KB) | Badger (1KB) | LocalStorage (1MB) | Badger (1MB) |
| :--- | :--- | :--- | :--- | :--- |
| **Latency** | 143,033 ns | **63,636 ns** | **338,388 ns** | 1,049,112 ns |
| **Throughput** | 7.16 MB/s | **16.09 MB/s** | **3,098.74 MB/s** | 999.49 MB/s |

> **Observation**: **BadgerDB** is surprisingly faster for small writes (1KB) because it writes to an in-memory MemTable (LSM Tree) before flushing to disk, whereas **LocalStorage** incurs immediate filesystem metadata overhead (syscalls) for every file creation. However, for large files (1MB), **LocalStorage** dominates purely on sequential I/O speed.

### 3. Read Performance

| Metric | LocalStorage (1KB) | Badger (1KB) | LocalStorage (1MB) | Badger (1MB) |
| :--- | :--- | :--- | :--- | :--- |
| **Latency** | 33,465 ns | **6,621 ns** | 97,009 ns | 123,373 ns |
| **Throughput** | 30.60 MB/s | **154.65 MB/s** | **10,809.02 MB/s** | 8,499.24 MB/s |

> **Observation**: Similar to writes, **BadgerDB** shines in small random reads (likely due to Block Cache or OS cache efficiency for its single data file), but **LocalStorage** is untouchable for large sequential reads, effectively operating at memory copy speeds when cached by the OS.

## Analysis

### LocalStorage (Native)
- **Strengths**: Unmatched raw throughput for large files (3GB/s Write, 10GB/s Read). Lowest overhead for Seek.
- **Best For**: Video streaming media files, large datasets, and when maximum raw I/O performance is required.

### BadgerDB (KV Store)
- **Strengths**: High performance for small files/records due to LSM tree structure (MemTable buffering). Portable single-file database.
- **Weaknesses**: Significant overhead for `Seek` operations (10x slower than native) due to chunk decoding and assembly.
- **Best For**: Distributed systems requiring portability, encryption-at-rest, or applications handling many small files where filesystem metadata overhead would be a bottleneck.

## Conclusion
- **General Use**: Default to **LocalStorage** for standard VFS workloads (media, logs).
- **Specialized Use**: Choose **BadgerDB** if you need a self-contained filesystem (e.g., specific export formats, encrypted stores) and can tolerate ~100 MB/s seek limits.
