# Sizing a rotating tracker

Use these numbers as a comparison baseline, not a service SLO or a supported requests-per-second limit. One operation here is **one RegisterRequest plus one ReportOutcome** on the same key. The benchmark always issues both calls (one failure per eight outcomes), regardless of the throttle decision. In a service, don't report a resource outcome for a request that never attempted to acquire the resource.

## Reproduce the reference

Run each command separately on an otherwise idle instance. For rollout decisions, repeat the throughput run with `-count=5` or more and compare the spread under identical CPU and memory limits.

```sh
GOMAXPROCS=4 GOGC=100 go test -buildvcs=false ./pkg/tracker -run '^$' -bench '^BenchmarkProduction$' -benchtime=1s -count=1 -benchmem
GOMAXPROCS=4 GOGC=100 go test -buildvcs=false ./pkg/tracker -run '^$' -bench '^BenchmarkAtLimitRotation$' -benchtime=1x -count=1 -benchmem
```

Measured once with Go 1.23.3 on linux/amd64, Intel Core i9-13900. The timed test uses four closed-loop workers, one shared tracker, L=3, M=2048, other algorithm parameters at defaults, and a real 25 ms rotation interval to exercise repeated replacement during load (not a recommended production cadence). Workers either hit the same 8-byte identifier or cycle through 4096 prebuilt identifiers with staggered offsets. Stats are configured per tracker, not switched on individual requests.

| Keys | IncludeStats | Pairs/s | ns/op | Sampled p99, µs/pair | B/op | Allocs/op | Sampled peak HeapAlloc, MiB | Promotions/run |
|---:|:---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | off | 693,315 | 1442 | 65.647 | 540 | 12 | 3.897 | 47 |
| 1 | on | 917,551 | 1090 | 70.028 | 713 | 16 | 3.838 | 42 |
| 4096 | off | 3,518,802 | 284.2 | 2.227 | 531 | 12 | 3.888 | 47 |
| 4096 | on | 3,192,671 | 313.2 | 2.520 | 707 | 16 | 4.067 | 49 |

`promotions/run` is the change in `RotationStatus().Rotations` across the parallel workload, not the number of scheduled ticks. If it is zero, increase `-benchtime` before interpreting the result as rotating traffic. Other tests ran on the host during this reference run: the hot-key rows show higher tail latency than the distributed rows, but their throughput difference with stats on versus off is not reliable evidence of a speedup. `ns/op` is parallel wall time divided by completed pairs, **not** individual latency. The p99 is nearest-rank elapsed time around both calls, sampled every `max(64, N/65536+1)` pairs per worker. It includes lock waits and scheduling but no upstream queueing; periodic sampling can miss stalls. Allocation rates include concurrent rotation and harness overhead, not tracker construction. The process-wide heap is sampled about every 10 ms; that instrumentation can perturb timing and miss the true high-water mark. It is neither retained tracker size nor RSS.

## Memory and rollout limits

Two structures serve traffic; construction of the replacement can temporarily require a third. The config guard is `3 × (4096 + L × (64 + 64 × M)) ≤ 64 MiB` **per tracker**, an estimate for bucket storage, not a process memory cap. At L=3, the largest admitted M is 116486 (integer division). The isolated `BenchmarkAtLimitRotation` constructed that pair and confirmed one promotion in 57.9 ms including polling, allocating **33,569,064 bytes** and 1,048,413 objects per build-and-rotate operation. That is cumulative allocation, not a measured pause or peak live heap. The difference from 64 MiB is not spare container memory.

* Before deployment, budget for overlapping generations across **all** trackers, request and stats allocations, unreclaimed old generations, and the rest of the process. Reserve additional space above the Go heap for stacks, native memory and runtime overhead. A dimension check alone cannot establish safe headroom.
* Benchmark the intended L/M, rotation cadence, key skew, stats setting and concurrency in the target container. Then replay peak arrivals and bursts (not just closed-loop traffic) through the full service across many rotations. Measure service p99, accepted load, rotation failures, GC CPU/pauses and container memory high-water against its hard limit. Check that heap and RSS settle rather than climbing over successive cycles.
* Set gates from the service's own latency budget, demand forecast and required memory reserve. Canary one instance before widening. Hold or roll back on rotation failures, sustained queue growth, tail-latency regression outside that budget, or loss of the required memory reserve. If stats are optional, test them off first; retest fairness before reducing dimensions to save memory.
