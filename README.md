# FAIR

FAIR (**F**air **A**llocation of **I**nsufficient **R**esources) is a Go library to add fairness to your service or application.

## Introduction

When you are serving a resource that's limited in some way (e.g. database records, place to run jobs, files/blobs etc.) to multiple clients at the same time and you are running out of the resource, you want to serve what you have fairly instead of say overallocating to a small number of clients and starving the rest. In distributed systems, implementing this sort of fairness requires careful thinking around what fairness means in your application, how to handle attempts to grab an unfare share and when to actually trigger any sort of throttling of requests. FAIR attempts to provide an interface agnostic of any web frameworks or protocols to track and enforce fairness. The library provides an easy to use throttler with opinionated configuration that can be further tuned to fit your needs.

The core algorithm of FAIR is based on the [Stochastic Fair BLUE](https://rtcl.eecs.umich.edu/rtclweb/assets/publications/2001/feng2001fair.pdf) algorithm often used for network congestion control with a few modifications. The philosophy of FAIR is to only throttle any client when there's a genuine shortage of resources as opposed to the approaches like token bucket or leaky bucket which may reject requests even when the resource is still available (a creative configuration of FAIR can enable that type of behavior but we don't encourage it). Since the state is stored in a multi-level [Bloom Filter](https://medium.com/p/e25942ab6093) style data structure, the memory needed is constant and does not scale with the number of clients. When properly configured, FAIR can scale to a very large number of clients

## Installation

To install the FAIR library, use `go get`:

```bash
go get github.com/satmihir/fair
```

Then, import it into your Go code:

```go
import "github.com/satmihir/fair"
```