# VFS Driver Benchmark Report

## Overview

This benchmark compares the BadgerDB and LocalStorage VFS drivers for create,
read, and seek workloads. The goal is to choose a backend based on measured
latency, throughput, and allocation cost rather than a single throughput value.

## Latest Environment

- **OS**: macOS (Darwin ARM64)
- **CPU**: Apple M4 Pro
- **Go Version**: 1.26.6
- **Commit**: `ee95386`
- **Test Date**: 2026-08-25

Record the OS, CPU, Go version, commit, power mode, and test date whenever the
results are refreshed. Results from different environments are not directly
comparable.

## Workloads

### Create

Creates a unique file on every iteration using 1KB, 4KB, or 1MB of zero-filled
data. Directory creation and path generation are excluded from the timer.

### Read

Repeatedly opens and fully reads the same 1KB, 4KB, or 1MB file. File creation
is excluded from the timer. This is primarily a warm-cache workload.

### Seek

Creates a 50MB file, then repeatedly seeks using the deterministic offset
`(i * 102400) % (size - 1024)` and reads 1KB. File creation and opening are
excluded from the timer. This simulates repeatable non-sequential access; it is
not a statistically random access pattern.

## Reproducible Comparison

Close CPU- and disk-intensive applications, use the same power mode, then run:

```sh
make benchmark
```

This runs every driver benchmark 10 times for at least 2 seconds, validates all
140 result rows, removes Badger log noise, and prints a `benchstat` summary.
The raw, normalized, and summary outputs are saved as:

- `/tmp/govfs-benchmark.raw`
- `/tmp/govfs-benchmark.txt`
- `/tmp/govfs-benchmark.stats`

To compare revisions, preserve each run under a different prefix and compare
the normalized files:

```sh
BENCHMARK_OUTPUT=/tmp/govfs-old make benchmark
# Switch to the candidate revision.
BENCHMARK_OUTPUT=/tmp/govfs-new make benchmark
go run golang.org/x/perf/cmd/benchstat@v0.0.0-20260819171926-ebcb4798430d \
  /tmp/govfs-old.txt /tmp/govfs-new.txt
```

Interpret a change as meaningful only when `benchstat` reports a statistically
significant difference. Also inspect `B/op` and `allocs/op`; lower latency that
causes materially higher allocation pressure may not improve production
performance. Rerun noisy results after checking thermal throttling, background
I/O, and Badger compaction activity.

## Latest Precision-Run Results

The following values are `benchstat` medians and ranges from 10 samples with a
2-second minimum duration per sample. The package runs took about 19 minutes.
The driver ratios are descriptive because the drivers use different benchmark
names; statistical significance applies when the same benchmark is compared
between revisions.

### Latency

| Workload | Size | LocalStorage | BadgerDB | Lower Latency |
| :--- | :--- | ---: | ---: | :--- |
| Create | 1KB | 148.1µs ± 4% | 56.53µs ± 2% | BadgerDB (2.62x) |
| Create | 4KB | 151.5µs ± 3% | 1.624ms ± 9% | LocalStorage (10.72x) |
| Create | 1MB | 494.3µs ± 13% | 972.9µs ± 8% | LocalStorage (1.97x) |
| Read | 1KB | 32.05µs ± 3% | 1.812µs ± 11% | BadgerDB (17.69x) |
| Read | 4KB | 31.13µs ± 1% | 2.414µs ± 5% | BadgerDB (12.90x) |
| Read | 1MB | 93.62µs ± 4% | 118.0µs ± 0% | LocalStorage (1.26x) |
| Seek + Read | 1KB | 762.4ns ± 10% | 10.34µs ± 1% | LocalStorage (13.56x) |

### Throughput

| Workload | Size | LocalStorage | BadgerDB |
| :--- | :--- | ---: | ---: |
| Create | 1KB | 6.590 MiB/s ± 4% | 17.28 MiB/s ± 2% |
| Create | 4KB | 25.78 MiB/s ± 3% | 2.408 MiB/s ± 10% |
| Create | 1MB | 1.976 GiB/s ± 11% | 1.004 GiB/s ± 8% |
| Read | 1KB | 30.47 MiB/s ± 3% | 538.8 MiB/s ± 10% |
| Read | 4KB | 125.5 MiB/s ± 1% | 1.580 GiB/s ± 5% |
| Read | 1MB | 10.43 GiB/s ± 4% | 8.279 GiB/s ± 0% |
| Seek + Read | 1KB | 1.251 GiB/s ± 9% | 94.49 MiB/s ± 1% |

### Allocation Cost

| Workload | Size | LocalStorage | BadgerDB |
| :--- | :--- | ---: | ---: |
| Create | 1KB | 3.203 KiB/op, 14 allocs/op | 93.94 KiB/op, 782 allocs/op |
| Create | 4KB | 3.323 KiB/op, 14 allocs/op | 13.48 MiB/op, 882.5 allocs/op |
| Create | 1MB | 3.091 KiB/op, 14 allocs/op | 21.58 MiB/op, 1,581 allocs/op |
| Read | 1KB | 642 B/op, 8 allocs/op | 5.357 KiB/op, 37 allocs/op |
| Read | 4KB | 642 B/op, 8 allocs/op | 14.36 KiB/op, 37 allocs/op |
| Read | 1MB | 642 B/op, 8 allocs/op | 3.005 MiB/op, 70 allocs/op |
| Seek + Read | 1KB | 1 KiB/op, 1 alloc/op | 302.3 KiB/op, 5 allocs/op |

## Current Interpretation

- **LocalStorage** remains the default for large files, streaming, sustained
  writes, and seek-heavy workloads. Its seek latency was about 13.6x lower.
- **BadgerDB** is competitive for small objects and was much faster for warm
  1KB and 4KB reads, but it allocates substantially more memory.
- BadgerDB's 4KB create median rose from 181µs in the smoke run to 1.624ms in
  the precision run. Longer runs expose sustained-write and compaction costs.
- Large 1MB reads were close, while LocalStorage retained an advantage for 1MB
  creates.
- Driver selection must consider latency and allocation cost together. The
  latest precision run is a baseline; a regression claim requires a second
  revision measured under the same conditions.

## Limitations

- Read and seek benchmarks repeatedly access one file, so OS and database cache
  effects dominate after warm-up.
- The data is zero-filled and does not represent compression, encryption, or
  application-level processing costs.
- The benchmark measures driver API behavior, not raw storage-device speed.
- Create results may be affected by BadgerDB background flushing and compaction.
- Durability under crashes, concurrent access, tail latency, and cold-cache
  behavior are not covered.

Add a new workload only when one of these limitations maps to a real production
requirement.
