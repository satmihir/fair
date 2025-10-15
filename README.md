# FAIR
![Coverage](https://img.shields.io/badge/Coverage-58.3%25-yellow)
[![Go Report Card](https://goreportcard.com/badge/github.com/satmihir/fair)](https://goreportcard.com/report/github.com/satmihir/fair)
[![GoDoc](https://godoc.org/github.com/satmihir/fair?status.svg)](https://godoc.org/github.com/satmihir/fair)

FAIR is a Go library designed to ensure fairness in resource‑constrained environments. It helps distribute limited resources evenly across multiple clients, preventing over‑allocation and starvation.

## Table of Contents
- [Introduction](#introduction)
- [Key Features](#key-features)
- [Evaluation](#evaluation)
- [Installation](#installation)
- [Quick Start](#quick-start)
  - [Registering Requests](#registering-requests)
  - [Reporting Outcomes](#reporting-outcomes)
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

## Installation

To install the FAIR library, use `go get`:

```bash
go get github.com/satmihir/fair
```

## Quick Start

### Basic Configuration

To use the default config which should work well in most cases:

```go
trkB := tracker.NewFairnessTrackerBuilder()

trk, err := trkB.BuildWithDefaultConfig()
defer trk.Close()
```

### Custom Configuration

If you want to make some changes to the config, you can use the setters on the builder:

```go
trkB := tracker.NewFairnessTrackerBuilder()
// Rotate the underlying hashes every one minute to avoid correlated false positives
trkB.SetRotationFrequency(1 * time.Minute)

trk, err := trkB.Build()
defer trk.Close()
```
Enabling Stats and Debugging Bucket Details
To collect and inspect per-bucket statistics for debugging, set IncludeStats to true in your tracker config:

```go

conf := config.DefaultFairnessTrackerConfig() //default IncludeStats=false
conf.IncludeStats = true
tracker, _ := tracker.NewFairnessTracker(conf)
```

ResultStats contains probabilities and other debugging information collected while registering a request.
After registering requests or reporting outcomes, you can access bucket stats from the result:

```go
result := tracker.RegisterRequest(ctx, clientID)
if result.Stats != nil {
    log.Printf("Bucket: %v, Requests: %d, Throttled: %d", 
        result.Stats.BucketID, result.Stats.RequestCount, result.Stats.ThrottledCount)
}
```

This helps you debug fairness decisions and monitor workload behavior.

### Registering Requests

For every incoming request, you have to pass the flow identifier (the identifier over which you want to maintain fairness) into the tracker to see if it needs to be throttled. A client ID for example could be such ID to maintain resource fairness among all your clients.

```go
ctx := context.Background()
id := []byte("client_id")

resp := trk.RegisterRequest(ctx, id)
if resp.ShouldThrottle {
    throttleRequest()
}
```

### Reporting Outcomes

For any failure that indicates a shortage of resource (which is our trigger to start throttling), you report outcome as a failure. For any other outcomes that are considered failures in your business logic that don't indicate resource shortage, do not report any outcome.

```go
ctx := context.Background()
id := []byte("client_id")

trk.ReportOutcome(ctx, id, request.OutcomeFailure)
```

On the other hand, when you are able to get the resource, you report success.

```go
ctx := context.Background()
id := []byte("client_id")

trk.ReportOutcome(ctx, id, request.OutcomeSuccess)
```

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

## Logging
Fair provides logs present which by default are disabled.
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
