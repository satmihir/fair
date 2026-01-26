# Design Document: State Service for Distributed FAIR

## Overview
FAIR instances maintain state variables (probability and last update time) that determine the outcome of requests. In a distributed deployment, all FAIR instances must converge on an eventually consistent view of these variables while jointly contributing to their updates.

This document describes a **State Service** that aggregates state from all FAIR instances and broadcasts updates to all connected clients.

### Goals
- Enable state sharing and consumption across all FAIR instances via a gRPC API.
- Clients will come to an eventually consistent view of the buckets provided they are able to commit their deltas to the State Service and fetch the state.
- Design for single-instance deployment initially, with a path to horizontal scaling.

## Background / Problem Statement
Distribute state across all active FAIR instances efficiently. The State Service acts as the centralized aggregation point, receiving deltas from all instances and broadcasting aggregated updates.

## Glossary
- **Seed**: An initial seed value used to initialize the hash function for a time window. Rotating the seed periodically ensures that flows (Client IDs) hashed to the same bucket in one window are likely to land on different buckets with the new seed.
- **Bucket**: A (row, column) cell containing probability and timestamp data.
- **Delta**: An incremental change to a bucket's probability and timestamp.

## Requirements

### Functional
- **Receive Deltas**: FAIR instances push local state deltas to the State Service, The state-service will aggregated the deltas
- **Broadcast**: All connected clients receive updates for all changed buckets.

### Non-Functional
- **Availability**: FAIR instances degrade gracefully to local state if the State Service is unavailable.
- **Scalability**: Single instance initially; design supports future horizontal scaling via replication.

### Latency / Performance Targets
- **Hot Path Impact**: Delta pushes from the hot path must be asynchronous and non-blocking (client-side concern).
- **Update Propagation**: Changed buckets are broadcast to all clients immediately upon aggregation by default; periodic batching (e.g., every 250ms) may be enabled for high-throughput scenarios.
- **Static Stability**: In case the state service is unavailable, FAIR instances should continue to operate using their local state.

## Out of Scope
- Client-side batching logic (how FAIR instances batch deltas before sending).
- Request evaluation logic (how state is used to evaluate requests).
- Hot Key Handling & Broadcast Batching: To avoid N² message amplification and hot-key starvation, the server may batch updates over a configurable window and prioritize broadcasts using randomized selection (e.g., Power-of-2 choices).

## Architecture

```
┌─────────────┐                           ┌─────────────────────────┐
│  FAIR (A)   │◄─────────────────────────►│                         │
├─────────────┤   Bidirectional gRPC      │     State Service       │
│  FAIR (B)   │◄────────Stream───────────►│  ┌───────────────────┐  │
├─────────────┤                           │  │  Aggregation      │  │
│  FAIR (C)   │◄─────────────────────────►│  │  Engine           │  │
└─────────────┘                           │  └───────────────────┘  │
                                          │  ┌───────────────────┐  │
                                          │  │  In-Memory Store  │  │
                                          │  └───────────────────┘  │
                                          │  ┌───────────────────┐  │
                                          │  │  Broadcast Hub    │  │
                                          │  └───────────────────┘  │
                                          └─────────────────────────┘
```

### Components

| Component | Responsibility |
|-----------|----------------|
| **Aggregation Engine** | Receives deltas, applies aggregation logic (sum probabilities, max-timestamp). |
| **In-Memory Store** | Stores current aggregated state per seed/bucket. Extensible to embedded DB. |
| **Broadcast Hub** | Maintains client connections, broadcasts changed buckets to all clients. |

---

## Data Model

### Bucket State
```
Key:   (seed, row_id, col_id)
Value: {
    probability: float64      // Aggregated probability
    lastUpdateTimeMs: uint64  // Max timestamp across all updates
}
```

### Aggregation Semantics
| Field | Aggregation Strategy |
|-------|---------------------|
| **Probability** | Additive — deltas are summed. **Clamped to [0.0, 1.0]** after aggregation. |
| **Timestamp** | Max-Timestamp-Wins — latest timestamp is retained. |

---

## gRPC API

### Service Definition

```protobuf
syntax = "proto3";

package fair.state.v1;

service StateService {
  // Bidirectional stream for delta submission and state reception.
  // Client sends deltas; server broadcasts aggregated bucket updates.
  rpc Sync(stream SyncRequest) returns (stream SyncResponse);
}

message SyncRequest {
  oneof request {
    DeltaUpdate delta_update = 1;
    StateRequest state_request = 2;  // Request full state for a seed
  }
}

message StateRequest {
  uint64 seed = 1;
}

message DeltaUpdate {
  uint64 seed = 1;
  repeated BucketDelta deltas = 2;
}

message BucketDelta {
  uint64 row_id = 1;
  uint64 col_id = 2;
  double delta_prob = 3;         // Increment/decrement value
  uint64 last_update_time_ms = 4;
}

message SyncResponse {
  uint64 seed = 1;
  repeated Bucket buckets = 2;   // Changed buckets only (sparse)
}

message Bucket {
  uint64 row_id = 1;
  uint64 col_id = 2;
  double prob = 3;               // Aggregated absolute value
  uint64 last_update_time_ms = 4;
}
```

### API Semantics

| Direction | Message | Behavior |
|-----------|---------|----------|
| **Client → Server** | `DeltaUpdate` | Server aggregates deltas into store, then broadcasts changed buckets to all clients. |
| **Client → Server** | `StateRequest` | Server returns all non-default buckets for the requested seed. Used for cold start and seed rotation. |
| **Server → Client** | `SyncResponse` | Contains buckets (sparse). Client overwrites local state. |

### Broadcast Behavior
- When a delta arrives, the server:
  1. Aggregates the delta into the in-memory store.
  2. Broadcasts the updated bucket(s) to **all** connected clients. This can either be done right away (or) periodically (every 250ms) based on the configuration.

---

## Cold Start & Seed Rotation
Clients explicitly request state when needed. This handles both cold start (new client) and seed rotation (new time window) uniformly.

### Protocol

Client sends `StateRequest{seed}` on the bidirectional stream:
- **Cold start**: Client connects, computes current seed, requests state.
- **Seed rotation**: Client detects new time window, computes new seed, requests state.

Server responds with `SyncResponse` containing all non-default buckets for that seed.

### Client Flow

```
1. COLD START
   ├── Connect to State Service
   ├── Compute current seed from local clock
   ├── Send: StateRequest{seed: current_seed}
   ├── Receive: SyncResponse{seed, buckets[]}
   └── Overwrite local state

2. STEADY STATE
   ├── Receive broadcasts: SyncResponse{seed, buckets[]}
   ├── Overwrite local state
   └── Send local deltas: DeltaUpdate{seed, deltas[]}

3. SEED ROTATION
   ├── Detect time window change (local clock)
   ├── Compute new seed
   ├── Send: StateRequest{seed: new_seed}
   ├── Receive: SyncResponse{seed, buckets[]}
   └── Overwrite local state for that seed
```

### Why Explicit (Not Implicit)?

| Approach | Problem |
|----------|--------|
| **Implicit (auto-send on connect)** | Server doesn't know client's clock/seed. Reconnecting clients receive redundant data. Doesn't help with seed rotation. |
| **Explicit (client requests)** | Client controls what it needs. Works for cold start AND seed rotation. Avoids wasted bandwidth. |

### Client Update Logic

Clients use **blind overwrite** for all incoming state from the server:

**Rationale:**
- The server is the authoritative source — it aggregates all deltas from all clients.
- Any local delta has already been sent to the server and will be reflected in future broadcasts.
- Temporary regression during the race window is acceptable for FAIR's probabilistic use case.
- Simpler logic to implement and reason about.

---

## Seed Lifecycle

### Seed Identification
Seeds are determined by **rounded local time** — each time window produces a deterministic seed based on the current timestamp rounded to the window duration. This also has the property that new seeds will be monotonically increasing helping with evicting stale entries.

**Assumption**: Clocks across FAIR instances are synchronized (or within acceptable skew < 10% of window duration).

### TTL & Expiry
- Buckets for stale seeds (older than `3 × window_duration`) are automatically evicted.
- Eviction is handled by a background goroutine in the State Service.
- Deltas received for evicted seeds are silently dropped.

---

## Failure Handling

| Scenario | Behavior |
|----------|----------|
| **State Service unavailable** | FAIR instances degrade to local state. Convergence stops, availability maintained. |
| **Client disconnect** | Server removes client from broadcast list. Client reconnects, sends StateRequest to resync, and resumes receiving updates. |
| **Backpressure / Overload** | gRPC flow control applies. Slow clients may miss intermediate updates but eventually receive latest state. |
| **Client unable to invoke API** | Clients will attempt to send deltas to the State Service with bounded retry logic; deltas are dropped after retry exhaustion. |

## Storage Backend

### Initial Implementation: In-Memory
- Simple Go map protected by `sync.RWMutex`.
- Keyed by `(seed, row_id, col_id)`.
- Suitable for single-instance deployment.

### Future: Embedded Database
- Extensible to BoltDB, Badger, or SQLite for persistence.
- Storage interface abstraction allows swapping backends without API changes.

```go
type Store interface {
    // Apply a delta and return the updated bucket state.
    ApplyDelta(seed, rowID, colID uint64, deltaProb float64, timestampMs uint64) Bucket

    // Get all buckets for a seed.
    GetSeed(seed uint64) []Bucket

    // Evict buckets older than the given seed.
    EvictBefore(seed uint64)
}
```

### Future: Horizontal Scaling

#### Replication Strategy
For horizontal scaling, the State Service can be replicated using a **Raft-based consensus** or **eventually consistent replication** via etcd or a gossip protocol.

> Note: Replication is a future enhancement. Initial deployment is single-instance.

---

## Security Considerations
- **TLS**: All gRPC connections must use TLS.
- **Authentication**: mTLS or token-based authentication for FAIR instances.
- **Authorization**: All authenticated clients have equal access (no per-client ACLs needed).

This is not covered in the doc and will be taken up in a future enhancement.
---

## Observability

| Metric | Description |
|--------|-------------|
| **Connected Clients** | Number of active gRPC streams. |
| **Deltas Received/sec** | Inbound delta rate. |
| **Buckets Broadcast/sec** | Outbound broadcast rate. |
| **Aggregation Latency** | Time from delta receipt to broadcast. |
| **Store Size** | Number of buckets in memory. |

## Implementation Plan
1. **Define Protobuf Schema**: Create `state.proto` with service and message definitions.
2. **Implement In-Memory Store**: Thread-safe store with aggregation logic.
3. **Implement Broadcast Hub**: Manage client streams, fan-out updates.
4. **Implement gRPC Server**: Wire up `Sync` RPC with store and hub.
5. **Integration**: Update FAIR instances to use State Service client.

### Callout
One minor change that needs to be addressed is the code to identify decays, it possible the l

## Testing
- **Unit Tests**: Store aggregation logic, broadcast fan-out.
- **Integration Tests**: Multiple clients pushing deltas, verifying all receive correct aggregated state.
- **Chaos Tests**: Client disconnects, server restarts, network partitions.

---

## Appendix: Race Condition Analysis

### The Race Window

A brief inconsistency can occur when a client's local delta is overwritten by a broadcast that doesn't yet reflect that delta.

**Timeline:**
```
Client                          State Service
  │                                   │
  ├─── Apply delta locally ───────────┤
  │    (local: 0.5 + 0.1 = 0.6)       │
  │                                   │
  ├─── Send delta to service ────────►│
  │                                   │
  │◄── Broadcast (0.5) ───────────────┤  (doesn't include our delta yet)
  │    (local overwrites to 0.5)      │
  │                                   │
  │◄── Broadcast (0.6) ───────────────┤  (now includes our delta)
  │    (local = 0.6) ✓                │
```

### Why This Is Acceptable

| Factor | Reasoning |
|--------|----------|
| **Race window is short** | Milliseconds between sending delta and receiving aggregated broadcast. |
| **Delta is not lost** | Server has received it and will broadcast the aggregated result. |
| **FAIR is probabilistic** | Temporary inconsistency in probability values has minimal impact on overall behavior. |
| **Simplicity wins** | No timestamp tracking or merge logic required on client. |