# Design Document: Distributing State Across All Peers

## Overview
FAIR instances maintain state variables (probability and last update time) that determine the outcome of requests. These variables are updated periodically based on traffic patterns.

In a distributed deployment, all FAIR instances must converge on an eventually consistent view of these variables while jointly contributing to their updates.

### Goals
- Enable state sharing and consumption across all FAIR instances.
- Ensure eventual consistency of shared state.

## Background / Problem Statement
Distribute state across all active FAIR instances efficiently.

## Glossary
- **Seed**: An initial seed value used to initialize the hash function for a time window. Rotating the seed periodically ensures that flows (Client IDs) hashed to the same bucket in one window are likely to land on different buckets with the new seed. This prevents a heavy flow from permanently penalizing a lighter flow due to hash collisions.

## Requirements

### Functional
- **Push**: Instances must be able to push local state deltas to the central store.
- **Pull**: Instances must be able to fetch the aggregated state from the central store.

### Non-Functional
- **Availability**: The system must continue to function (using local state) even if the central store is unavailable.
- **Scalability**: The solution should support multiple FAIR instances updating concurrently.
- **Consistency**: The system guarantees eventual consistency for the shared state.

### Latency / Performance Targets
- **Hot Path Impact**: Updates from the hot path must be asynchronous and non-blocking.
- **Convergence**: State should converge across instances within a reasonable time window (dependent on polling interval).

## Out of Scope
- Defining the logic for how state is used to evaluate requests.

## Design
Aggregated state is maintained in a centralized store. Instances commit their local deltas to this store and periodically pull the latest aggregated state.

We have two types of data:
1. **Seed**: The current hash seed for the time window.
2. **Bucket**: The probability and timestamp data.

Bucket data has high cardinality (Space = `NumSeeds * NumCols * NumRows`). Each bucket contains a probability value and a "last seen" timestamp.

### Seed Identification Strategy
To ensure all instances agree on the seed for a given time window, three options were evaluated:

1.  **Commit Start Time**: Instances use distributed locks to agree on a global start time. They arrive at the seed based on the difference between the current time and the start time.
2.  **Rounded Local Time**: Instances use their local time, rounded to the window duration.
3.  **Computed Seed**: Instances compute a monotonically increasing seed and coordinate via distributed locks.

We chose **Option 2 (Rounded Local Time)**.
- **Assumption**: Clocks across instances are synchronized.
- **Benefit**: Simplifies design by avoiding distributed locks and complex coordination.

### Data Storage & Schema

#### Storage Service
A `Storage Service` layer wraps the underlying KV store to decouple the application logic from storage implementation. This service handles batching and ensures updates do not block the hot path.

- **Hot Path**: APIs are invoked asynchronously.
- **Synchronization**: A background process ensures updates are committed to the central store.

#### Key Schema
We use **Redis Hashes** to optimize for storage efficiency and bulk retrieval.

**Schema:**
```text
Key:   v1:{seed}:{row_id}
Field: {col_id}:prob  -> Value: {probability} (float)
Field: {col_id}:time  -> Value: {timestamp}   (int)
```

**Update Semantics:**
- **Probability**: Uses `HINCRBYFLOAT` for atomic incremental updates.
- **Timestamp**: `Max-Timestamp-Wins` strategy - implemented by a Lua Script/Transaction to perform a compare and swap on the variable. This might introduce additional latency when compared with the `Last-Writer-Wins` strategy.

#### Service Contract
```go
// BucketDelta represents a change to a specific bucket
struct BucketDelta {
    rowID            uint64
    colID            uint64
    deltaProb        float64 // Increment/Decrement value
    lastUpdateTimeMs uint64  // Timestamp of update
}

// Update contains the seed and a batch of bucket changes
struct Update {
    seed    uint64
    updates []BucketDelta
}

// OverwriteBucket represents the absolute state of a bucket
struct OverwriteBucket {
    rowID            uint64
    colID            uint64
    prob             float64
    lastUpdateTimeMs uint64
}

// RespBucket contains the aggregated state for a seed
struct RespBucket {
    seed       uint64
    updates    []OverwriteBucket
}

// StorageService API - All APIs are non-blocking
interface StorageService {
    // Async request for buckets. Triggers background fetch.
    Request(ctx context.Context, seed uint64)

    // Returns a channel that receives updates.
    Recv(ctx context.Context) <-chan []RespBucket

    // Commit local deltas to storage.
    Update(ctx context.Context, updates []Update) error
}
```

### Sharing the State
FAIR instances commit the deltas to the centralized store. 

### Consuming the state
Instances periodically `Request` the state they want to consume and `Recv` the aggregated state from the Storage Service. These states are then applied to the local state.

### Optimization & Reliability

#### Batching
The Storage Service batches updates to prevent overwhelming the centralized store with network round-trips.

#### TTL & Memory Management
All keys created in the central store are set with an appropriate Time-To-Live (TTL) to automatically expire old data. The Keys can be set with 3x the time window duration, the rationale here being beyond the 3x TTL the keys that are being used will have lost its use.

#### Failure Model
- **Redis Unavailability**: If the centralized store is unreachable, FAIR instances degrade gracefully by functioning with their local state. Convergence stops, but availability is maintained. Any transient failures are retried before the updates are dropped. 

## Alternatives Considered
**Peer-to-Peer Communication**: A P2P model without a centralized state was considered. This is not pursued at this time due to the implementation complexity.

## Security / Privacy Considerations
- The centralized store must be secured (e.g., TLS, Authentication) to prevent tampering with FAIR state.
- Credentials are injected into the instance at startup.

## Dependencies / Compatibility
- Centralized store (Redis) running on the same network.
- Redis FQDN and credentials injected into FAIR instance at startup.

## Implementation Plan
1.  **Infrastructure**: Deploy Redis alongside FAIR instances.
2.  **Storage Service**: Implement the service with the API defined above, including batching (pipelines).
3.  **Integration**: Hook the service into the FAIR event loop.

## Testing
- **Mock Testing**: Use Redis mocks to verify service logic.
- **Convergence Testing**: Simulate a cluster with random writes and measure the time required for all instances to converge on the same state.

## Monitoring & Observability
- **Redis Connectivity**: Monitor connection status and errors.
- **Update Latency**: Track the time taken to commit batches to Redis.
- **Queue Depth**: Monitor the size of the pending update queue in the Storage Service.

## Future Enhancements

### Cold Start Optimization
To improve performance for new instances, we can maintain a "start seed" in the central store. New instances can pull this aggregated history immediately upon startup rather than building state from scratch.

### Alternative Storage
Support for other storage backends or peer-to-peer state sharing can be added if the centralized Redis approach becomes a bottleneck.

### Recovery from failures
Currently the logic for applying the aggregated state on to local instance  always involve an overwrite. In certain cases, when the local state is more valuable, overwriting the aggregated state may not be the best behavior. This can be mitigated by introducing a weight - this can help defining additional squashing behavior.

