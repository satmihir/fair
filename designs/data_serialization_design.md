# FAIR Data Serialization Design
## Overview

This document outlines the design for serializing FAIR’s core data structures, which is essential for enabling FAIR as a distributed system. It also covers how serialized data is expected to be consumed and synchronized across hosts. 

This document doesn't capture details about how the serialized states from other hosts are merged/considered in the decision making.

## Glossary

What's a Bucket? \
The most basic data-structure in FAIR implementatin is a bucket - this contains a probability and a last-seen time. The probability is decayed (or) improved based on some factors. For the life-time of the Structure, a client - identified by the client-ID, consistently lands on a few buckets (based on seed). The final decision is made on top of the bucket probabilities.


## Requirements
### Functional

- **Complete**: Serialize the full data structure, including:
    - Multi-level bucket data (probabilities, timestamps)
    -  Configuration parameters
    - MurmurHash seed
    - Metadata (ID, flags)

- **Portable**: Work across platforms, Go versions, and FAIR versions, with proper versioning.
- **Compact**: Add minimal overhead for efficient transfer.
- **Future-proof**: Support schema evolution as new parameters are added.

### Non-Functional
- **Performance**: Fast serialization/deserialization.
- **Reliability**: Strong validation and error handling.
- **Maintainability**: Clear versioning and schema upgrade path.
- **Compatibility**: Backward and forward compatibility.

## Serialization Format Selection

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

Accessing `State` requires the matching `Config` and `Consts`. Multiple tracker configs may exist simultaneously (multi-tenant).

## Schema v1 (High-level)
```
package fair.data.v1;

enum LevelSquashingFunction {
  LEVEL_SQUASHING_FUNCTION_UNSPECIFIED = 0;
  LEVEL_SQUASHING_FUNCTION_MIN = 1;
  LEVEL_SQUASHING_FUNCTION_MEAN = 2;
}

enum HostSquashingFunction {
  HOST_SQUASHING_FUNCTION_UNSPECIFIED = 0;
  HOST_SQUASHING_FUNCTION_LOGIC1 = 1;
  HOST_SQUASHING_FUNCTION_LOGIC2 = 2;
}

message TrackerCfg {
  int64 config_guid = 1; // Unique per tracker config
  int64 config_version = 2; // Monotonically increments per version of tracker config
  uint32 m = 3;
  uint32 l = 4;
  double pi = 5;
  double pd = 6;
  double lambda = 7;
  google.protobuf.Duration rotation_frequency = 8;
  LevelSquashingFunction level_squash_fn = 9;
  HostSquashingFunction host_squash_fn = 10;
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

message FairRunConsts {
  uint32 murmur_seed = 1;
  string ref_guid = 2;
}

message HostMeta {
  string host_guid = 1;
  uint64 serialized_at_ms = 2;
}

message FairRunTimeData {
  FairRunConsts runtime = 1;
  FairData data = 2;
}

message FairSt {
  // int32 version_num = 1; May be do this only if necessary
  repeated TrackerCfg cfgs = 2;
  repeated FairRunTimeData data = 3;
  HostMeta meta = 4;
}
```

The top-level struct contains a list of `TrackerCfgs`, the Runtime data is encoded using `FairRunTimeData` - which contains the consts and the data. Each unit of the run-time data contains a link `ref_guid` which refers to the config_guid and version. The structure also include a `HostMeta` field that includes information about the host.


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
- Optional compression (gzip/lz4).
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

## Thoughts on State Syncing
Two approaches considered:
1) Independent host state
    - Each host keeps separate shadow structures for other hosts.
    - Simple but expensive (memory + fan-out peformance).
2) Seed Coordination
    - Predictable seeds via GetNextSeed() per time window.
    - Hosts converge on consistent bucket mappings.
    - More efficient but requires careful design to avoid feedback loops.