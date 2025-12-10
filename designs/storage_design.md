# Design Document: State Service for Distributed FAIR

## Overview
FAIR instances maintain state variables (probability and last update time) that determine the outcome of requests. In a distributed deployment, all FAIR instances must converge on an eventually consistent view of these variables while jointly contributing to their updates.

This document describes a **State Service** that aggregates state from all FAIR instances and broadcasts updates to all connected clients.

### Goals
- Enable state sharing and consumption across all FAIR instances via a gRPC API.
- Ensure eventual consistency of shared state.
- Design for single-instance deployment initially, with a path to horizontal scaling.

## Background / Problem Statement
Distribute state across all active FAIR instances efficiently. The State Service acts as the centralized aggregation point, receiving deltas from all instances and broadcasting aggregated updates.

## Glossary
- **Seed**: An initial seed value used to initialize the hash function for a time window. Rotating the seed periodically ensures that flows (Client IDs) hashed to the same bucket in one window are likely to land on different buckets with the new seed.
- **Bucket**: A (row, column) cell containing probability and timestamp data.
- **Delta**: An incremental change to a bucket's probability and timestamp.

## Requirements

### Functional
- **Push Deltas**: FAIR instances push local state deltas to the State Service.
- **Receive Aggregated State**: FAIR instances can request for specific aggregate state - which will then be delivered to the FAIR instances.
- **Broadcast**: All connected clients receive updates for all changed buckets.

### Non-Functional
- **Availability**: FAIR instances degrade gracefully to local state if the State Service is unavailable.
- **Scalability**: Single instance initially; design supports future horizontal scaling via replication.
- **Consistency**: Eventual consistency for the shared state.

### Latency / Performance Targets
- **Hot Path Impact**: Delta pushes from the hot path must be asynchronous and non-blocking (client-side concern).
- **Update Propagation**: Changed buckets are broadcast to all clients immediately upon aggregation.
- **Hot Key Prioritization**: Frequently updated buckets may be prioritized in broadcast ordering.

## Out of Scope
- Client-side batching logic (how FAIR instances batch deltas before sending).
- Request evaluation logic (how state is used to evaluate requests).

---

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
| **Probability** | Additive — deltas are summed. |
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
  }
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
| **Server → Client** | `SyncResponse` | Contains only buckets that changed since last broadcast. Sparse updates — client merges with local state. |

### Broadcast Behavior
- When a delta arrives, the server:
  1. Aggregates the delta into the in-memory store.
  2. Broadcasts the updated bucket(s) to **all** connected clients.
- **Hot Key Prioritization**: Buckets with higher update frequency may be prioritized in broadcast ordering.

---

## Seed Lifecycle

### Seed Identification
Seeds are determined by **rounded local time** — each time window produces a deterministic seed based on the current timestamp rounded to the window duration.

**Assumption**: Clocks across FAIR instances are synchronized (or within acceptable skew < 10% of window duration).

### TTL & Expiry
- Buckets for stale seeds (older than `3 × window_duration`) are automatically evicted.
- Eviction is handled by a background goroutine in the State Service.

---

## Failure Handling

| Scenario | Behavior |
|----------|----------|
| **State Service unavailable** | FAIR instances degrade to local state. Convergence stops, availability maintained. |
| **Client disconnect** | Server removes client from broadcast list. Client reconnects and resumes receiving updates. |
| **Backpressure / Overload** | gRPC flow control applies. Slow clients may miss intermediate updates but eventually receive latest state. |

---

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

---

## Future: Horizontal Scaling

### Replication Strategy
For horizontal scaling, the State Service can be replicated using a **Raft-based consensus** or **eventually consistent replication** via etcd or a gossip protocol.

> Note:Replication is a future enhancement. Initial deployment is single-instance.

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

---

## Implementation Plan
1. **Define Protobuf Schema**: Create `state.proto` with service and message definitions.
2. **Implement In-Memory Store**: Thread-safe store with aggregation logic.
3. **Implement Broadcast Hub**: Manage client streams, fan-out updates.
4. **Implement gRPC Server**: Wire up `Sync` RPC with store and hub.
5. **Integration**: Update FAIR instances to use State Service client.

---

## Testing
- **Unit Tests**: Store aggregation logic, broadcast fan-out.
- **Integration Tests**: Multiple clients pushing deltas, verifying all receive correct aggregated state.
- **Chaos Tests**: Client disconnects, server restarts, network partitions.

---

## Appendix: Race Condition Mitigation

### The Problem

A race condition can occur when a client's local delta is overwritten by a broadcast that doesn't yet reflect that delta.

**Timeline:**
```
Client                          State Service
  │                                   │
  ├─── Apply delta locally ───────────┤
  │    (bucket X: +0.1)               │
  │                                   │
  ├─── Send delta to service ────────►│
  │                                   │
  │◄── Broadcast for bucket X ────────┤  (from another client's earlier update)
  │    (overwrites local state)       │
  │                                   │
  │    X Local delta lost             │
```

**Why this matters:**
- The broadcast may contain state from a concurrent update by another client.
- This concurrent state could be more or less valuable than the local delta.
- Blindly overwriting loses the client's contribution until the server eventually broadcasts the aggregated result.

### Mitigation Strategies

| Strategy | Description |
|----------|-------------|
| **Timestamp comparison** | Only apply broadcast if `broadcast.timestamp > local.timestamp`. Ensures newer local updates are not overwritten by stale broadcasts. |
| **Request count weighting** | Carry a count of requests contributing to each bucket. Clients can weight the broadcast value against local state based on contribution counts. |
| **Commutative updates** | Since the probabilities are overwritten always allow probability increments but do not allow probability decrements. This ensures that the broadcasted value is always the latest aggregated value. |

### Recommended Approach

Start with **timestamp comparison** — simple and effective for most cases. The State Service always broadcasts the **latest aggregated value**, so any temporary inconsistency resolves quickly once the delta is applied server-side and re-broadcast.