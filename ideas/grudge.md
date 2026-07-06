# grudge: A Decaying-Score Sketch

**Status:** Idea / draft spec
**Author:** Mihir Sathe
**Date:** 2026-07-05

> **grudge** — a constant-memory sketch that holds grudges and forgives:
> decaying feedback scores over unbounded key spaces. FAIR is fair because
> grudge forgets.

## Motivation

FAIR's throttling algorithm sits on top of a data structure that is more
general than throttling: a constant-memory sketch that maps an unbounded key
space to a continuously-decaying scalar score, updated by external feedback.
It is to behavioral scores what count-min sketch is to counts.

This spec extracts that structure into a standalone library, `grudge`, so
that FAIR (and other projects: cost-aware LLM-gateway fairness, fair job
schedulers, abuse scoring, per-key circuit breakers, cache admission) can
depend on it. FAIR itself becomes a thin policy layer over grudge (see
[Appendix A](#appendix-a-fair-re-expressed-on-grudge)).

Three properties from FAIR are preserved as the core value proposition:

1. **Collision shielding** — L independent hash levels with a configurable
   aggregator (min by default) so a key colliding with a "hot" key on one
   level is exonerated by its other levels.
2. **Decay** — scores decay exponentially toward zero so no key is penalized
   forever; decay is computed lazily so there is no background work.
3. **Rotation** — periodic reseeding of the hash functions so collision sets
   change over time, making false positives transient rather than persistent.

## Terminology

- **Key**: an arbitrary byte string identifying a flow (client ID, tenant ID,
  cache key, ...). The key space is unbounded; keys are never stored.
- **Score**: a float64 associated with a key, clamped to a configured range.
  Interpretation is entirely up to the caller (throttle probability, abuse
  score, priority penalty, ...).
- **Cell**: one (level, index) slot holding `(score, lastUpdatedMillis)`.
- **Generation**: one complete L×M array of cells under a single hash seed.
- **Aggregator**: function collapsing a key's L per-level scores into one.

## Core semantics

### The ideal (reference) model

Ignoring collisions, each key `k` has a true score `S(k)` evolving as:

```
S(k, t) = clamp( decay(S(k, t₀), t−t₀) + Σ deltas in (t₀, t] )
```

- `decay` is a pluggable function that must be monotone toward zero and
  compose across time subdivision: `decay(decay(s, t₁), t₂) = decay(s, t₁+t₂)`.
  Built-ins:
  - `Exponential(λ)` — `s · e^(−λ·Δt)`; FAIR's decay, proportional forgiveness.
  - `Linear(r)` — `max(0, s − r·Δt)`; constant-rate forgiveness, the token
    bucket refill (see [ratelimiter.md](ratelimiter.md)).
  - `None` — scores persist until explicitly decremented or rotated out.
- `clamp(x)` bounds to `[Lo, Hi]` (FAIR uses `[0, 1]`; a scheduler might use
  `[0, +∞)` via `Hi = +Inf`).
- Deltas are applied after decaying the stored value to "now".

### The sketch approximation

The structure stores L levels of M cells each. Key `k` maps to one cell per
level via L hashes derived from a single 64-bit murmur3 hash of `k` (the
Kirsch–Mitzenmacher construction, as in FAIR today). Reads and writes touch
exactly L cells.

- **Update(k, δ)**: for each of k's L cells: lazily decay the cell to now,
  then add δ and clamp.
- **Query(k)**: lazily decay each of k's L cells to now, then return
  `Aggregate([s₁ … s_L])`.

**Error model** (documented, not enforced): a query returns the true score
plus contamination from colliding keys. With min-aggregation the estimate is
one-sided — `Query(k) ≥ S(k)` never underestimates by more than decay
drift — and overestimates only when *all* L cells collide with hot keys,
probability `(1 − (1 − 1/M)^H)^L` for H hot keys. This is FAIR's existing
`CalculateL` math, which moves into grudge's tuning helper.

## API sketch (Go)

```go
package sketch

// Config for a single sketch (one generation).
type Config struct {
    Levels        uint32  // L ≥ 1
    CellsPerLevel uint32  // M ≥ 1
    Decay         Decay   // decay function; Decay = None disables decay
    Lo, Hi        float64 // clamp range; Lo < Hi; Hi may be +Inf when decay
                          // and merge semantics permit (see Validation)
    Aggregator    Aggregator
    Clock         Clock   // injectable for tests/simulation; default real
}

// Decay evolves a score over elapsed idle time. Implementations must be
// monotone toward zero and compose across time subdivision:
//   Apply(Apply(s, t1), t2) == Apply(s, t1+t2)
// (required for lazy evaluation and for the mergeability law below).
type Decay interface {
    Apply(score float64, dt time.Duration) float64
}

// Built-in decays.
func Exponential(lambda float64) Decay // s·e^(−λ·Δt); FAIR's decay
func Linear(ratePerSec float64) Decay  // max(0, s − r·Δt); token-bucket refill
var None Decay                         // identity; no decay

// Aggregator collapses per-level scores into one. Built-ins: Min, Mean, Max.
type Aggregator func(levelScores []float64) float64

// Sketch is one generation: L×M cells under one seed.
type Sketch struct { /* unexported */ }

func New(cfg Config) (*Sketch, error)
func NewWithSeed(cfg Config, seed uint32) (*Sketch, error) // deterministic, for tests/replication

// Update decays k's cells to now, then adds delta (clamped). Thread-safe.
func (s *Sketch) Update(key []byte, delta float64)

// Query decays k's cells to now and returns the aggregated score. Thread-safe.
// Note: Query writes back the decayed values (lazy decay), so it takes the
// same per-cell locks as Update.
func (s *Sketch) Query(key []byte) float64

// QueryDetailed also returns per-level scores and cell indexes (FAIR's
// IncludeStats, made a call-site choice instead of a config flag).
func (s *Sketch) QueryDetailed(key []byte) Result

// UpdateAndQuery performs Update then returns the post-update aggregate in a
// single pass over the L cells (one lock acquisition per cell instead of two).
func (s *Sketch) UpdateAndQuery(key []byte, delta float64) float64

// TryUpdate applies delta only if the post-decay aggregate plus delta stays
// within limit, returning whether it was applied. This is the atomic
// conditional-consume needed for strict quota semantics (see ratelimiter.md):
// Query-then-Update is racy under concurrency for the same key. Unlike the
// other operations, TryUpdate holds all L cell locks simultaneously — still
// deadlock-free because every operation acquires locks in level order — so it
// costs more contention than the optimistic path; callers that can tolerate
// small over-admission should prefer Query + Update.
func (s *Sketch) TryUpdate(key []byte, delta float64, limit float64) bool

func (s *Sketch) Seed() uint32
func (s *Sketch) Config() Config
```

### Rotation: the generational wrapper

FAIR's main/secondary pair generalizes to N generations. This lives in
grudge because rotation is part of what makes the sketch's false positives
transient — but as a separate type so single-generation users don't pay for
it.

```go
// Rotator manages N generations of Sketch under changing seeds.
// Reads serve from the oldest ("primary") generation; writes apply to all
// generations so younger ones are warm when they become primary.
type Rotator struct { /* unexported */ }

type RotatorConfig struct {
    Sketch      Config
    Generations uint32        // ≥ 2; FAIR today uses 2
    Period      time.Duration // rotation interval
    Ticker      Ticker        // injectable; default real
}

func NewRotator(cfg RotatorConfig) (*Rotator, error)
func (r *Rotator) Update(key []byte, delta float64)
func (r *Rotator) Query(key []byte) float64
func (r *Rotator) UpdateAndQuery(key []byte, delta float64) float64
func (r *Rotator) TryUpdate(key []byte, delta float64, limit float64) bool // decides on primary, applies to all
func (r *Rotator) Close() // stops the rotation goroutine
```

Rotation semantics: every `Period`, drop the primary, promote the next
generation, append a fresh sketch with a new random seed. A key's worst-case
false-positive exposure is bounded by `Generations × Period`.

### Merge and serialization (distribution hooks)

These two operations are what the distributed state-service design depends
on; grudge treats them as first-class.

```go
// MergeDelta folds another sketch's *deltas* into this one. Both sketches
// must share (Levels, CellsPerLevel, seed, clamp range). Cell-wise:
//   score = clamp(decay(local) + delta.score)
//   lastUpdated = max(local, delta.lastUpdated)
func (s *Sketch) MergeDelta(d *SketchDelta) error

// Snapshot / Restore round-trip the full structure (protobuf on the wire).
func (s *Sketch) Snapshot() (*SketchSnapshot, error)
func FromSnapshot(snap *SketchSnapshot) (*Sketch, error)

// DeltaSince returns cells modified after the given time, for incremental sync.
func (s *Sketch) DeltaSince(t time.Time) *SketchDelta
```

**Mergeability law**: because updates are additive and decay composes across
time subdivision (exponential: `e^(−λt₁)·e^(−λt₂) = e^(−λ(t₁+t₂))`; linear:
`max(0, s − r·t₁ − r·t₂)` away from the zero boundary — this composition
requirement is exactly why the `Decay` interface demands it), two replicas
that exchange deltas converge to the same cell values regardless of
interleaving, up to clamp effects. Clamping breaks strict commutativity at
the boundaries (a cell pinned at `Hi` absorbs updates that a different
ordering would have preserved); this is documented as acceptable drift,
consistent with the existing storage design's "blind overwrite" stance.

## Semantics details

### Lazy decay

Each cell stores `(score float64, lastUpdatedMillis uint64)`. On every access
(read or write), before anything else:

```
Δt   = nowMillis − lastUpdatedMillis
score = decay.Apply(score, Δt)
lastUpdatedMillis = now
```

No background goroutine ever touches cells. An idle sketch does zero work.
With `Exponential(λ)` this is inherited unchanged from FAIR's `visitBuckets`.

### Concurrency

- Per-cell mutex, as today. Update/Query lock one cell at a time in level
  order and never hold two locks, so no deadlock and no cross-level
  atomicity. A concurrent reader may observe level i post-update and level
  i+1 pre-update — acceptable for the same reason FAIR accepts it: the
  structure is probabilistic and single-cell linearizability suffices.
- `TryUpdate` is the one exception: it acquires all L cell locks (in level
  order, so still deadlock-free against every other operation), computes the
  aggregate, and conditionally applies — trading contention for an atomic
  read-check-write across levels.
- Rotator uses an RWMutex around the generation slice exactly as FAIR's
  tracker does today (updates are readers; rotation is the writer).
- Possible future optimization (out of scope for v1): pack
  `(float32 score, uint32 ageSeconds)` into a single atomically-CAS'd uint64
  per cell, eliminating mutexes. Noted here so the cell layout stays private.

### Clamping and validation

- `Lo < Hi` required. `Lo = 0, Hi = 1` with `Exponential(λ)` reproduces FAIR.
- `Hi = +Inf` is allowed **only when** decay is not `None` (unbounded scores
  with no decay would grow without bound) and is incompatible with
  `MergeDelta`'s convergence guarantee at the boundary (trivially true — there
  is no boundary).
- Clamp-range guidance per decay: with `Linear(r)`, `Hi` bounds worst-case
  recovery time to `Hi/r` seconds (see ratelimiter.md for why `Hi = B` vs
  `Hi > B` is a semantic choice); with `Exponential(λ)`, recovery half-life
  is `ln2/λ` regardless of `Hi`.
- `Levels ≥ 1`, `CellsPerLevel ≥ 1`, `Decay` non-nil (use `None` explicitly).
- Unlike FAIR today, grudge imposes **no** constraint between increment
  and decrement magnitudes (Pi > Pd is throttling policy, not structure
  invariant). It moves up into FAIR.

### Tuning helper

```go
// SuggestDimensions returns (L, M) achieving a target all-levels-collision
// probability p given an expected count of concurrently "hot" keys H.
// Solves p = (1 − (1 − 1/M)^H)^L  — FAIR's CalculateL, relocated.
func SuggestDimensions(hotKeys uint32, cellsPerLevel uint32, p float64) (L uint32, M uint32)
```

Pi/Pd derivation from `tolerableBadRequestsPerBadFlow` stays in FAIR — it is
about the meaning of scores, not the structure.

## What grudge explicitly does NOT include

- **Decision policy**: no notion of throttle/allow, success/failure, Pi/Pd,
  or randomized decisions (`rand.Float64() <= p`). Callers interpret scores.
- **Transport**: no gRPC, no state service; only Merge/Snapshot/Delta hooks.
- **Key storage**: keys are hashed and discarded; the sketch cannot enumerate
  keys. Callers needing top-k must track candidates externally.
- **Windowing/quotas**: no token buckets, no rate math.

## Package / repo shape

New repo `satmihir/grudge` (Go module `github.com/satmihir/grudge`):

```
sketch/         core Sketch type, aggregators, lazy decay
rotate/         Rotator
mergepb/        protobuf schema for Snapshot/Delta (v1)
tune/           SuggestDimensions and the collision math
internal/hash/  murmur3 + Kirsch–Mitzenmacher n-hash derivation
```

`satmihir/fair` then depends on grudge; `pkg/data` becomes an adapter that
wires Pi/Pd/final-probability policy onto `rotate.Rotator` (a major-version
change for FAIR, or an internal refactor keeping FAIR's public API intact —
the latter is possible since grudge is a strict generalization).

## Open questions

1. **Weighted decay per level?** Some use cases may want a different `Decay`
   per level (fast + slow memory, like multi-timescale EWMAs). v1 says no —
   one `Decay` for the whole sketch — but the config shape shouldn't
   preclude it.
2. **Float32 cells?** Halves memory; probably enough precision for scores.
   Decide before v1 since it leaks into the protobuf schema.
3. **Rotator read policy**: FAIR reads only the primary generation. An
   alternative is aggregating across generations (e.g., max) to smooth the
   promotion cliff. Needs the eval harness ported over to answer empirically.
4. **Negative scores**: `Lo < 0` enables "credit" semantics (a key banks
   goodwill). Decay toward zero pulls negative scores *up* — is that the
   right semantic, or should decay pull toward `Lo`? v1: decay always pulls
   toward 0 and requires `Lo ≤ 0 ≤ Hi`.

## Appendix A: FAIR re-expressed on grudge

| FAIR concept | grudge expression |
|---|---|
| Bucket probability | Cell score with `Lo=0, Hi=1` |
| `ReportOutcome(Failure)` | `Update(k, +Pi)` |
| `ReportOutcome(Success)` | `Update(k, −Pd)` |
| `RegisterRequest` | `p := Query(k)`; throttle iff `rand() ≤ p` |
| `FinalProbabilityFunction` (min/mean) | `Aggregator` (Min/Mean) |
| Lambda decay in `visitBuckets` | `Exponential(λ)` lazy decay (identical math) |
| Main/secondary + rotation goroutine | `Rotator{Generations: 2}` |
| `IncludeStats` config flag | `QueryDetailed` call-site variant |
| `GenerateTunedStructureConfig` L calc | `tune.SuggestDimensions` |
| Serialization design doc (v1.pb.go) | `Snapshot`/`FromSnapshot` in `mergepb` |
| Storage design doc's delta aggregation | `MergeDelta` / `DeltaSince` |

One behavioral delta worth noting: FAIR's `RegisterRequest` today *writes*
(it refreshes decay timestamps on the read path) — with grudge this is
just `Query`, whose lazy decay has the same effect. Bit-for-bit compatibility
is expected except for RNG sequencing.
