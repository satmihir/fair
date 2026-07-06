# Rate Limiting on grudge: Constant-Memory Token Buckets over Unbounded Keys

**Status:** Idea / draft spec
**Author:** Mihir Sathe
**Date:** 2026-07-05
**Depends on:** [grudge.md](grudge.md) — specifically `Linear` decay and `TryUpdate`

## Pitch

A per-key token-bucket rate limiter for an **unbounded key space in constant
memory**: no per-key state, no eviction, no background work, O(L) per request.
A few MB covers millions of keys. The error is one-sided by construction —
*exact for the well-behaved, approximate for the abusive, bounded for
everyone.*

Incumbents pay for per-key exactness in the dimension that actually hurts:

- **Hashmap of buckets** (golang.org/x/time/rate per key, etc.): ~50+ bytes
  per key, unbounded growth, and LRU eviction machinery — where eviction *is*
  the failure mode, since evicting an active attacker's bucket resets their
  limit.
- **Redis-based limiters**: a network hop on the request hot path, plus the
  same per-key memory problem, just outsourced.

## Mechanics

### The representation inversion: track debt, not tokens

A sketch cannot initialize per-key state — a never-seen key reads whatever a
fresh cell holds, which is 0. So 0 must mean "full allowance," which forces
the inversion: the score is **tokens spent** (debt), not tokens remaining.

A classic bucket `(capacity B, refill r tokens/sec)` becomes a grudge sketch
with:

| Grudge config | Value | Meaning |
|---|---|---|
| `Decay` | `Linear(r)` | debt drains at the refill rate |
| `Lo, Hi` | `0, B` | debt can't go negative or exceed capacity |
| `Aggregator` | `Min` | collision shielding (see error model) |
| score | tokens spent | fresh key → 0 → full bucket, for free |

**GCRA equivalence**: a cell `(spent, lastUpdated)` under linear decay is
isomorphic to GCRA's theoretical arrival time (`TAT ≡ lastUpdated + spent/r`)
— the algorithm inside most production Redis rate limiters. This is GCRA run
through a sketch instead of a per-key map.

### Per-request flow

```
spent := sketch.Query(key)          // lazily decays k's L cells, min-aggregate
if spent + cost > B:
    reject                          // no debit — rejected requests don't consume
else:
    sketch.Update(key, +cost)       // debit all L cells
    allow
```

- **Variable cost is native**: `cost` is tokens for an LLM call, bytes for
  bandwidth, 1 for plain QPS.
- **Rejected requests don't debit** (token-bucket semantics). Retry-storm
  punishment, if wanted, is one line of policy: `Update(key, +ε)` on reject.

### Concurrency: optimistic vs strict

Query-then-Update is not atomic: two concurrent requests for the same key can
both see `spent = B − cost` and both pass, over-admitting by the concurrency
width.

- **Default (optimistic)**: accept it. Over-admission by a few requests is
  noise next to sketch collision error, and matches FAIR's single-cell
  linearizability stance.
- **Strict mode**: `sketch.TryUpdate(key, +cost, B)` — the atomic
  conditional-consume added to grudge for exactly this. Holds all L cell
  locks; pay the contention only if you need hard quotas.

### Clamp choice: `Hi = B` vs `Hi > B`

- `Hi = B`: debt can't exceed capacity, so even a maximally abusive key
  recovers to a full bucket in at most `B/r` seconds. Standard token bucket.
- `Hi > B` (up to `+Inf`): leaky-bucket-style "queue debt" — burst overspend
  extends the punishment window beyond the burst itself. Useful when
  over-limit *attempts* should push recovery further out. Note: only debits
  that pass admission accumulate; combine with a reject-penalty ε if
  hammering while limited should extend the window.

Both are legitimate; expose it as config.

## Error model

### Per-key error is one-sided (permissive)

With min-aggregation, a key's estimated spend is the min over its L cells;
collisions can only *under*-count. Consequences:

- An innocent key is **never throttled below its entitled rate** unless all
  L of its cells are contaminated by heavy keys — probability
  `(1 − (1 − 1/M)^H)^L` for H concurrently-heavy keys. `tune.SuggestDimensions`
  (FAIR's `CalculateL`) sizes L and M for a target.
- An adversary rotating keys reads fresh-ish cells and can exceed its per-key
  limit — this is "approximately enforced per-key limits," not a hard quota.

### The emergent aggregate backstop

Rotation attacks don't fail open. Every admitted request debits L cells, and
each level drains at most `M·r` tokens/sec in aggregate. So the sketch has a
built-in global ceiling of **~M·r tokens/sec per level**: sustained
key-rotation saturates cells broadly, every key's min rises, and the limiter
degrades into a coarse *aggregate* limiter rather than admitting unbounded
traffic.

Sizing rule: choose `M·r` at or above total intended system capacity so the
backstop only engages under genuine abuse. The degradation mode does punish
innocents (their L cells all fill) — which is exactly what generation
rotation heals: a saturated generation ages out and collision sets reshuffle.

### Rotation and warmth

Grudge's dual-write rotation carries over and matters more here: writes go to
all generations, so the promoted generation already carries recent debt —
rotation must not hand every heavy spender a fresh bucket. Sizing rule:
`Period ≫ B/r`, so the debt horizon a new generation misses is small relative
to what linear decay would have forgiven anyway.

## API sketch (Go)

```go
package ratelimit // import "github.com/satmihir/grudge/ratelimit"

type Config struct {
    Rate  float64 // r: tokens per second
    Burst float64 // B: bucket capacity in tokens

    // Sketch sizing; zero values → tune.SuggestDimensions defaults.
    Levels        uint32
    CellsPerLevel uint32

    // MaxDebt > Burst enables queue-debt semantics; 0 → Burst (standard).
    MaxDebt float64

    // Strict uses TryUpdate (atomic, more contention) instead of the
    // optimistic Query+Update path.
    Strict bool

    RotationPeriod time.Duration // must be ≫ Burst/Rate; 0 → derived default
}

type Limiter struct { /* wraps a grudge rotate.Rotator */ }

func New(cfg Config) (*Limiter, error)

// Allow is AllowN(key, 1).
func (l *Limiter) Allow(key []byte) bool

// AllowN admits iff the key has cost tokens of headroom, debiting if so.
func (l *Limiter) AllowN(key []byte, cost float64) bool

// Spent returns the current debt estimate for a key (observability).
func (l *Limiter) Spent(key []byte) float64

func (l *Limiter) Close()
```

Multi-window limits (e.g., 10/s and 1000/h) compose as two Limiters with
different `(Rate, Burst)`; admit iff both allow. (Open question: a debit
applied to limiter A but rejected by limiter B should ideally be refunded
from A — refund interacts with clamping; v1 can accept the small leak.)

## Sizing worked example

L=4, M=100k, float64 cells: `4 × 100k × 16B ≈ 6.4 MB` for an unbounded key
space. With r=100 tokens/sec per key, the aggregate backstop is
`M·r = 10M tokens/sec` per level. Collision math for H=1000 simultaneously
heavy keys: per-level contamination `1−(1−1/M)^H ≈ 1%`, all-4-levels
`≈ 10⁻⁸` — innocents effectively never see false throttling.

## Where this could live

Options, not mutually exclusive:

1. `github.com/satmihir/grudge/ratelimit` — a subpackage; the flagship recipe
   proving the core library.
2. A standalone repo depending on grudge, if it grows integrations
   (HTTP middleware, gRPC interceptor, Envoy ext_authz / WASM).

Start as (1); split when integrations arrive.

## Open questions

1. **Refund on multi-limiter rejection** (see multi-window note above) —
   accept the leak, or add a `Refund(key, cost)` that documents its clamp
   interaction?
2. **Reject-penalty default**: should hammering-while-limited extend the
   window by default (safer under retry storms) or stay pure token-bucket
   (principle of least surprise)? Leaning: pure by default, ε-penalty opt-in.
3. **Per-key rate overrides**: heterogeneous limits (tenant tiers) don't fit
   one sketch since r is baked into the decay. Options: one sketch per tier
   (clean, bounded), or store normalized debt (spend/B) — the latter distorts
   merge semantics; leaning per-tier sketches.
4. **Naming**: does this ship as grudge's `ratelimit` package or earn its own
   name as a product?
