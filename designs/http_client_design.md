# Design: FAIR-Augmented HTTP Client

## Overview

We want a production-grade HTTP client integration for FAIR so that any given Go application(s) can benefit from client-side load shedding and fair resource distribution without bespoke wiring around every `http.Client`.

This design proposes:

* A **`FairRoundTripper`** that wraps any `http.RoundTripper` and adds FAIR.
* A **`FairHTTPClient`** that exposes a ready-to-use `http.Client` with FAIR, retry, and sane transport defaults.

The goal is: *“take an existing HTTP client, drop in FAIR, and get correctness + fairness + resilience with minimal code.”*

## Problem / Motivation

Today, FAIR is easy to use on the server side, but HTTP clients that call FAIR-protected services need to:

* Manually call `RegisterRequest` / `ReportOutcome` around each request.
* Re-implement standard client concerns: retry, connection pooling, timeouts.
* Invent their own patterns for how FAIR and retry interact.

This leads to:

* **Inconsistent integrations**: everyone wires it differently.
* **Higher friction to adoption**: more boilerplate than “just use FAIR”.

We want a standard, idiomatic way to “augment” HTTP clients with FAIR now, and a pattern we can generalize to other client types later (gRPC, WebSockets, etc.).

## Goals / Non-Goals

**Goals**

* Provide a **drop-in wrapper** (`FairRoundTripper`) for existing `http.Client`s.
* Provide a **standalone client** (`FairHTTPClient`) with production defaults.
* Make FAIR integration **idiomatic Go** (RoundTripper-based, composable).
* Handle **client ID extraction** and **FAIR outcome reporting** automatically.
* Include **basic resiliency**: retry with exponential backoff + idempotency awareness.
* Keep **overhead low** (target <5% P99 latency increase vs raw `http.Client` for non-throttled requests).

**Non-Goals / Out of Scope (for this design)**

* Server-side middleware changes.
* gRPC, WebSocket, or other non-HTTP transports (future designs).
* Circuit breakers, metrics/tracing, request hedging, client-side load balancing (future work).

## Proposed Design

### Package & Types

New package: `pkg/fairclient`.

Core types (high-level):

```go
// FairRoundTripper wraps an http.RoundTripper with FAIR.
type FairRoundTripper struct {
    config: *FairRoundTripperConfig
    transport: http.RoundTripper
}

// Config for FairRoundTripper.
type FairRoundTripperConfig struct {
    Tracker           *tracker.FairnessTracker          // required
    ClientIDExtractor ClientIDExtractor                 // optional, default header-based
    FallbackClientID  []byte                            // optional
    OnThrottle        func(*http.Request, *request.RegisterRequestResult)
    ThrottleStatusCode int                              // default 429
    RetryConfig       *RetryConfig                      // optional
}

// Production-ready client with FAIR built in.
type FairHTTPClient struct {
    client *http.Client
}

// Configures FairHTTPClient.
type FairHTTPClientConfig struct {
    FairConfig      *FairRoundTripperConfig // required
    Timeout         time.Duration           // default 30s
    TransportConfig *TransportConfig        // connection pooling defaults
}
```

Additional helpers:

* `type ClientIDExtractor func(*http.Request) ([]byte, error)`
* `HeaderClientIDExtractor(headerName string) ClientIDExtractor`
* `ContextClientIDExtractor(key any) ClientIDExtractor`
* `HashedClientIDExtractor(inner ClientIDExtractor) ClientIDExtractor`

### Behavior

**Request flow**

On every request:

1. **Extract client ID** with `ClientIDExtractor` (header by default, e.g. `X-Client-ID`).

   * If extraction fails and `FallbackClientID` is set, use fallback.
   * If extraction fails and no fallback, return an error immediately.

2. **Register with FAIR**

   * Call `Tracker.RegisterRequest(ctx, clientID)`.

3. **Throttle if needed**

   * If `ShouldThrottle` is true:
     * Option A (default): return a synthetic HTTP response with configurable status (default 429), `X-Fair-Throttled: true`, optional `Retry-After`.
     * Call `OnThrottle` hook if set.
   * No network call is made in this case.

4. **Execute request** using the wrapped `http.RoundTripper`.

5. **Report outcome**
   * Map `(resp, err)` into `request.Outcome` (success vs resource failure) and call `Tracker.ReportOutcome(ctx, clientID, outcome)`.

**Retry interaction**

* Retries are **optional**, configured via `RetryConfig`.
* Default behavior:
  * Retry only **idempotent methods** (`GET`, `HEAD`, `OPTIONS`, `TRACE`, `PUT`, `DELETE`).
  * Retry on:
    * Network errors.
    * Configurable 5xx codes (default: 500, 502, 503, 504) and optionally 429.
  * Use exponential backoff with jitter, with small, conservative defaults (e.g. max 3 attempts).

> Important: **Each retry attempt** registers and reports its own outcome with FAIR, so client behavior remains visible to the fairness algorithm and can be throttled if necessary.

**Standalone client**

`FairHTTPClient`:

* Constructs an `http.Transport` with production defaults:
  * Higher `MaxIdleConns` / `MaxIdleConnsPerHost` than `stdlib` defaults.
  * Sensible connection and header timeouts.
* Wraps that transport with `FairRoundTripper`.
* Exposes:
  * `Do(req *http.Request) (*http.Response, error)`
  * Convenience methods `Get`, `Post`, etc., delegating to the underlying `http.Client`.

This lets users choose:

* **Wrapper usage** (minimal change to existing code), or
* **New client usage** (get FAIR + better transport defaults in one go).

**Fail-Open vs Fail-Closed Behavior**

If the client is unable to communicate with the FAIR tracker (e.g., local process issue, shared-memory corruption, or unexpected internal errors), we must choose between:

#### Fail-Open (default)

If FAIR errors, allow the request to proceed as a normal HTTP request, without fairness metadata.

Rationale:
- Prioritizes availability over fairness.
- Prevents widespread outages if FAIR has a local transient issue.
- Keeps FAIR as non-critical path for correctness.

#### Fail-Closed (optional)

If FAIR errors, return an error immediately and do not send the request.

Rationale:
- Prioritizes strict fairness guarantees.
- Useful for environments where all traffic must be fairness-aware (internal-only systems, research testing, academic correctness verification).

>
> **Proposed Default: Fail-Open**
> The majority of production use cases favor availability: if FAIR stops functioning momentarily, the system should degrade gracefully rather than blocking all outbound requests. Fail-closed remains available for advanced users who need stronger correctness guarantees.
>

### Security / Privacy

**Client identifiers**

* Document that **client IDs must not contain PII** (e.g. raw emails, password, tokens etc.)
* Recommend using **opaque IDs or hashes**.
* Provide `HashedClientIDExtractor` helper that wraps another extractor and returns a _SHA-256_ hash of the ID.

FAIR already stores identifiers in a Bloom filter rather than as plain strings, but we still encourage non-PII identifiers for defense-in-depth.

**Retry amplification**

* Defaults are intentionally conservative:
  * Small `MaxAttempts` (e.g. 3).
  * Exponential backoff with jitter.
  * Only idempotent methods retried by default.

* Document guidance on:
  * When to enable retries.
  * How to tune backoff under load.

**TLS / certificates**

* We rely entirely on `net/http` defaults for TLS.
* We **do not** expose any “disable verification” helper APIs.
* If users need custom TLS config, they construct their own `*http.Transport` and wrap it with `FairRoundTripper`.

### Non-Functional Expectations

* **Performance:**
  * Target: wrapper logic adds <1ms overhead and <5% P99 latency increase in non-throttled scenarios.
* **Concurrency:**
  * `FairRoundTripper` and `FairHTTPClient` are safe to share across goroutines.
* **Dependencies:**
  * No new non-stdlib dependencies; relies on `github.com/satmihir/fair/pkg/...` and `net/http`.

## Alternatives Considered

1. **Wrap `http.Client` instead of `RoundTripper`**
   * Rejected: less composable and deviates from idiomatic Go HTTP middleware.
2. **Code generation from OpenAPI**
   * Rejected: heavy, brittle, and doesn’t help users with existing clients.
3. **Only provide a standalone `FairHTTPClient`**
   * Rejected: forces migration; wrapper is important for incremental adoption.

## Implementation & Testing Plan (High-Level)

**Implementation steps**

1. Add `pkg/fairclient` package.
2. Implement `FairRoundTripper`:
   * Config + constructors.
   * FAIR integration (`RegisterRequest` / `ReportOutcome`).
   * Throttling behavior.
3. Implement optional retry logic (`RetryConfig`).
4. Implement `FairHTTPClient` and `TransportConfig` helpers.
5. Add basic examples and README section.

**Testing**

* Unit tests:
  * Client ID extraction (header/context/hash).
  * Throttle behavior and hooks.
  * Outcome mapping for different status codes / errors.
  * Retry logic (idempotency, backoff, max attempts).
* Integration tests:
  * End-to-end against a simple FAIR-protected test server.
  * Verify that throttling reduces load under synthetic overload scenarios.
* Performance tests (basic benchmarks):
  * Compare Fair-wrapped vs raw `http.Client` for latency and allocations.

## Future Work

* **Circuit breaker RoundTripper** stacked with FAIR and retry.
* **Metrics / tracing RoundTripper** (Prometheus + OpenTelemetry).
* **Request hedging / advanced retry policies** for tail-latency-sensitive clients.
* **Generalization to other clients** (gRPC interceptors, WebSocket wrappers).
* **Framework integrations** (Gin/Echo/Chi helpers).
