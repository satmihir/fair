# FAIR Data Serialization Design
## Overview

This document outlines the design for serializing FAIR’s core data structures, which is essential for enabling FAIR as a distributed system. It doesn't cover how the serialized data is expected to consumed across hosts.


## Glossary 

What's a Bucket? \
The most basic data-structure in FAIR implementation is a bucket - this contains a probability and a last-seen time. The probability is increased (or) decreased based on success/failure and it decays over time by a configurable factor. For the life-time of the Structure, a client - identified by the client-ID, consistently lands on a few buckets (based on seed). The final decision is made on top of the bucket probabilities.

## Requirements
### Functional
- **Complete**: Serialize the full data structure, including:
    - Multi-level bucket data (probabilities, timestamps)
    - Configuration parameters
    - Algorithm / Seed
    - Metadata (ID, flags)

- **Portable**: Work across platforms, Go versions, and FAIR versions, with proper versioning.
- **Compact**: Add minimal overhead for efficient transfer.
- **Future-proof**: Support schema evolution as new parameters are added.

### Non-Functional
- **Performance**: Fast serialization/deserialization.
- **Reliability**: Strong validation and error handling.
- **Maintainability**: Clear versioning and schema upgrade path.
- **Compatibility**: Backward and forward compatibility.

## Serialization - Design
### Options
Rejected options: JSON, XML, and other text formats — too verbose. \
Custom serialization: Considered, but unnecessary given current requirements. Can revisit if protobuf overhead becomes a concern. \
Chosen option: Protocol Buffers (protobuf), due to:
- Binary format
- Native support in gRPC
- Strong typing and schema evolution
- Built-in JSON support (for debugging)
- Schema Design

The schema has two broad parts:
1)  Config (TrackerCfg) — defined by customers, drives tracker creation.
2) Runtime Data — driven by traffic and execution:
   - Consts: Initialized once, updated periodically (e.g., hash seed).
   - State: Updated frequently on the hot path (bucket data).

Accessing `State` requires the matching `Config` and `Consts`. Multiple tracker configs may exist simultaneously.

### Schema v1 (High-level)
```
package fair.data.v1;

enum LevelSquashingFunction {
  LEVEL_SQUASHING_FUNCTION_MIN = 0;
  LEVEL_SQUASHING_FUNCTION_MEAN = 1;
}

message TrackerCfg {
  int64 tracker_id = 1; // Unique per tracker config
  int64 config_version = 2; // Monotonically increments per version of tracker config
  uint32 m = 3;
  uint32 l = 4;
  double pi = 5;
  double pd = 6;
  double lambda = 7;
  google.protobuf.Duration rotation_frequency = 8;
  LevelSquashingFunction level_squash_fn = 9;
}

message Bucket {
  double probability = 1;
  uint64 last_updated_time_ms = 2;
}

message Level {
  repeated Bucket buckets = 1;
}

message FairData {
  repeated Level levels = 1;
}

enum Algorithm {
  MURMURHASH_32b = 0;
}

message AlgoParams{
  Algorithm algorithm = 1;
  uint32 murmur_seed = 2;
}

message FairRunConsts {
  AlgoParams algoparams = 1;
}

message HostMeta {
  string host_guid = 1;
  uint64 serialized_at_ms = 2;
}

message FairRunTimeData {
  FairRunConsts runtime = 1;
  FairData data = 2;
}

// FairStruct - wraps the configuration and the run-time consts & data associated with a tracker.
message FairStruct {
  TrackerCfg cfg = 1;
  FairRunTimeData data = 2;
  HostMeta meta = 3;
}
```

`FairStruct` wraps a tracker config and the associated consts and data. 

## Implementation Plan
### Phase 1: Core

Define schema (proto/fair/data/v1/structure.proto).
Generate Go code with protoc-gen-go.
Implement:
```
func (s *Structure) Serialize() ([]byte, error)
func DeserializeStructure(data []byte) (*Structure, error)
```

### Phase 2: Integration
- Add serialization tests
- Validate deserialized data
- Robust error handling
- Benchmark performance 
- Support human readable dumping (JSON)

### Phase 3: Enhancements
- Optional compression (gzip/lz4/zstd).
- Checksums for integrity.
- Migration utilities for schema upgrades.

``` 
type SerializationOptions struct {
    Compress        bool
    IncludeChecksum bool
    SchemaVersion   uint32
}

func (s *Structure) Serialize(opts *SerializationOptions) ([]byte, error)
func DeserializeStructure(data []byte) (*Structure, error)
func DeserializeStructureWithOptions(data []byte, opts *DeserializationOptions) (*Structure, error)
func ValidateStructure(s *Structure) error
```

Error handling includes structured errors (SerializationError) and standard cases (invalid data, unsupported schema, checksum mismatch, etc.).

## Performance Considerations

For a typical config (L=5, M=1000):
- Raw: ~40KB (5 levels × 1000 buckets × 8 bytes).
- With 100 trackers: ~4MB.

Compression and deltas can reduce size further — to be validated with experiments.

## Testing
Correctness: Round-trip serialization tests. \
Performance: Benchmarks for speed and memory use.

## Monitoring & Observability
Metrics: serialization latency, data size, error rates, schema usage.\
Logging: events, errors, warnings (e.g., slow serialization).

## Future Enhancements
1) Compression.
2) Delta syncs (share only updated fields).
3) External cache (Redis) for global state.
4) gRPC + service discovery for host coordination.
5) Warm-up mechanisms for new hosts.