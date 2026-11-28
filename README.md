# FAIR
[![Go Report Card](https://goreportcard.com/badge/github.com/satmihir/fair)](https://goreportcard.com/report/github.com/satmihir/fair)
[![GoDoc](https://godoc.org/github.com/satmihir/fair?status.svg)](https://godoc.org/github.com/satmihir/fair)

FAIR is a Go library designed to ensure fairness in resource‑constrained environments. It helps distribute limited resources evenly across multiple clients, preventing over‑allocation and starvation.

## Table of Contents
- [Introduction](#introduction)
- [Key Features](#key-features)
- [Evaluation](#evaluation)
- [Installation](#installation)
- [Quick Start](#quick-start)
  - [Request lifecycle](#request-lifecycle)
  - [Inspecting stats](#inspecting-stats)
- [Tuning](#tuning)
- [Resources](#resources)
- [License](#license)

## Introduction

The core algorithm of FAIR is based on the [Stochastic Fair BLUE](https://rtcl.eecs.umich.edu/rtclweb/assets/publications/2001/feng2001fair.pdf) often used for network congestion control with a few modifications. The philosophy of FAIR is to only throttle when there's a genuine shortage of resources as opposed to the approaches like token bucket or leaky bucket which may reject requests even when the resource is still available (a creative configuration of FAIR can enable that type of behavior, but we don't encourage it). Since the state is stored in a multi-level [Bloom Filter](https://medium.com/p/e25942ab6093) style data structure, the memory needed is constant and does not scale with the number of clients. When properly configured, FAIR can scale to a very large number of clients with a low probability of false positives and a near zero probability of persistent false-positives. The hash rotation mechanism regularly rehashes clients to avoid any correlated behavior longer than a few minutes.

### Key Features

- Framework and protocol agnostic and easy to integrate into any HTTP/GRPC service.
- Automatic tuning with minimal configuration out of the box with flexibility to fully tune if needed.
- Scalable to large numbers of clients with constant memory requirements.
- A simple resource and error tracking model that can be easily morphed into many types of throttling scenarios.

### Evaluation

![Evaluation](eval.png)

In this example, 20 clients are competing for a resource that regenerates at the rate of 20/s (every data point in the graph is 5s apart). 18 out of 20 clients are "well-behaved" because they request a resource every second while the remaining two clients try to get a resource every 100ms which is an "unfair" rate. On the left, we see that when left unthrottled, the two unfair clients grab a disproportionately large amount of resource while the regular workloads starve and get a lot less than 1/s rate. On the right, when throttled with fair, the regular workloads stay virtually unaffected while the unfair ones get throttled. On average, even the unfair workloads get their fair share when seen over larger time periods.

The deterministic evaluator in `cmd/fairness_eval` compares a shared token bucket with and without FAIR. It reports Jain's fairness index over each client's allocation relative to its max-min-fair entitlement, plus resource utilization as a throughput guardrail.

```bash
go run ./cmd/fairness_eval
```

Headline results with the default FAIR configuration:

| Scenario | No FAIR Jain | FAIR Jain | Improvement | Utilization with FAIR |
| --- | ---: | ---: | ---: | ---: |
| API noisy neighbors | 0.2184 | 0.6621 | +203.2% | 99.97% |
| Batch surge | 0.7497 | 0.9970 | +33.0% | 89.34% |

For the API noisy-neighbor workload, a focused FAIR configuration sweep found materially higher fairness. The figures in this paragraph are historical: they come from an earlier run of the sweep and have not been regenerated, so do not compare them with the table above. In that run the strongest practical point used `MinFinalProbabilityFunction`, `tolerableBadRequestsPerBadFlow=60`, `Lambda=0`, and at least 1,500 buckets per level, reaching a Jain score of 0.8498 with 99.28% utilization, a +289.1% improvement over no FAIR.

[![FAIR noisy-neighbor visual demo](demo/fairness_visual/fair-demo-thumbnail.png)](demo/fairness_visual/fair-demo.mp4)

A browser-based visual demo for the noisy-neighbor case is available in `demo/fairness_visual`. Click the preview above to open the MP4.

The [multi-seed fairness gate](cmd/fairness_eval/README.md) checks equal-client and noisy-neighbor traffic across rotations. For request cost and rotation headroom, see the [benchmark and rollout guide](designs/production-performance.md).

## Installation

To install the FAIR library, use `go get`:

```bash
go get github.com/satmihir/fair/pkg/tracker
```

## Quick Start

The [runnable quick starts](pkg/tracker/example_test.go) include imports and expected output. From the repository root, run both with:

```bash
go test ./pkg/tracker -run '^Example' -v
```

### Request lifecycle

Follow `ExampleFairnessTracker` in the [example source](pkg/tracker/example_test.go):

1. Build one tracker for the shared resource at startup. `NewFairnessTrackerBuilder` starts with defaults; apply any setters before `Build`. Check the build error before using the tracker, and close it at shutdown.
2. Before attempting to acquire the resource, call `RegisterRequest` with a stable flow ID (for example, a tenant ID). If `ShouldThrottle` is true, reject the request immediately. FAIR makes the decision; your application enforces it. No resource was attempted, so do not report an outcome.
3. Otherwise, try to acquire the resource. With the same flow ID, report `request.OutcomeSuccess` if you obtained it or `request.OutcomeFailure` if a shortage prevented acquisition. A validation error or network timeout by itself establishes neither: report nothing. Any other outcome value is ignored.

### Inspecting stats

See `ExampleFairnessTrackerBuilder_SetIncludeStats` in the [example source](pkg/tracker/example_test.go). Call `SetIncludeStats(true)` before `Build` to inspect individual decisions. `RegisterRequest` returns optional `ResultStats` (nil by default): `FinalProbability` is the probability used for the decision, and `BucketIndexes` and `BucketProbabilities` identify the selected bucket and its probability at each level. These are not per-client request or throttle counts.

## Tuning

You can use the `GenerateTunedStructureConfig` to tune the tracker without directly touching the algorithm parameters. It exposes a simple interface where you have to pass the following things based on your application logic and scaling requirements.
- `expectedClientFlows` - Number of concurrent clients you expect to your app
- `bucketsPerLevel` - Number of buckets per level in the core structure
- `tolerableBadRequestsPerBadFlow` - Number of requests we can tolerate before we fully shut down a flow.

```go
conf, err := config.GenerateTunedStructureConfig(1000, 1000, 25)
if err != nil {
    log.Fatal(err)
}
trkB := tracker.NewFairnessTrackerBuilder()

trk, err := trkB.BuildWithConfig(conf)
if err != nil {
    log.Fatal(err)
}
defer trk.Close()
```

Tracker construction takes a snapshot of the config, so you can reuse a builder or change the config after a successful build. A custom probability function is copied as a function value: it must still be safe to call concurrently. Its result is clamped to `[0, 1]`; if it returns NaN, the tracker uses the smallest bucket probability for that decision and logs the first occurrence. Do not change the config concurrently with construction.

For manual configurations, use positive dimensions, `0 < Pd < Pi <= 1`, a finite nonnegative `Lambda` (zero disables decay), and a non-nil probability function. `NewFairnessTracker` also needs a positive rotation interval; a manually supplied ticker controls its own cadence. Check construction and tuning errors before using the tracker.

**Allocation limit:** All constructors and the tuner reject dimensions whose estimated peak bucket storage exceeds `config.MaxRotationBytes` (64 MiB per tracker). The estimate is `3 × (4096 + L × (64 + 64 × M))` bytes: two live structures plus the replacement built on rotation, with allowances for rows, bucket objects, and pointers. This is a guardrail on dimensions, not a cap on process RSS: leave headroom for GC, concurrent requests, other trackers, and the application. If tuning exceeds the limit, revisit the expected flow count and bucket count rather than relying on a partial allocation.

**Client identifiers:** Identifiers are placed with SipHash-2-4 under a secret 128-bit key that is redrawn from `crypto/rand` for every generation, so a client that chooses its own identifier cannot aim it at another client's buckets. Every level's bucket is derived from that one 64-bit hash. Two clients with different hashes share a bucket at every level about as often as chance allows (roughly `1/M^L`); two identifiers with the same hash share every level, which bounds the rate at about `2^-64` per pair however many levels are configured. If key generation fails, construction returns an error, and a failed rotation is retried as described under [Logging](#logging).

For a reproducible simulation, set `HashKeySource` on the config to a seeded reader; leave it nil in production. It controls placement only: each throttling decision still draws from `math/rand`'s package-level generator.

## Logging
Logging is disabled by default. If rebuilding a generation fails, the tracker keeps serving with its current pair and retries on the next tick. Poll `trk.RotationStatus()` for monitoring: `Failures` is cumulative and `LastError` clears after the next successful rotation. Neither requires enabling a logger. The retry applies to errors returned by construction, not process-level out-of-memory failures.
package `logger` exposes an interface with `GetLogger` and `SetLogger` methods.

It also provides an out of the box logger based on std lib. Here's how you can enable it:
```go
import (
	"github.com/satmihir/fair/pkg/logger"
)
logger.SetLogger(logger.NewStdLogger())
```

## Development

Run tests and static analysis locally with:

```bash
make test
```

Generate protobuff wrappers with:
```bash
make proto
```

Ensure [golangci-lint](https://github.com/golangci/golangci-lint) is installed to execute the linter.

## Resources

- [Medium post about FAIR](https://medium.com/p/8c3a54ecee35)

## License

FAIR is released under the [MIT License](LICENSE).
