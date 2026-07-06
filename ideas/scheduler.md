# Fair Scheduling on grudge: Stochastic Fair Queueing for Job Queues

**Status:** Idea / draft spec
**Author:** Mihir Sathe
**Date:** 2026-07-05
**Depends on:** [grudge.md](grudge.md)
**See also:** [ratelimiter.md](ratelimiter.md), [crazy_ideas.md](crazy_ideas.md)

## Pitch

Multi-tenant fair scheduling as a **shim on the queue you already run** —
not a new queue. Constant memory over unbounded tenant cardinality, no
per-tenant queues, no configured shares, work-conserving, and
collision-shielded so small tenants are never penalized for a heavy
neighbor's behavior.

The problem: tenant A enqueues 1M jobs, tenant B enqueues 1; B waits hours
behind A in any FIFO. Every incumbent fix — per-tenant queues + round-robin,
Temporal fairness keys, shuffle sharding — requires owning the queue
topology and carrying per-tenant state that grows with tenant count.

Lineage: networking solved this as **Stochastic Fair Queueing** (SFQ). FAIR
is SFB brought to application resources; this is SFQ brought to job queues,
with grudge's multi-level min-shielding, which classic SFQ never had (it
relied on hash perturbation alone). *WFQ quality at Bloom filter prices, on
the queue you already run.*

## The structural insight: selection vs. scoring

Scheduling asks "of everything waiting, whom do I serve next?" — a
*selection* question the sketch structurally cannot answer (keys are hashed
and discarded; no enumeration). The design trick: **let the queue do the
enumeration and the sketch do the judging.** Jobs flow past the dequeue
point in the queue's native order; the sketch answers, per job, "is this
tenant currently over its share?" — and the queue's own deferral mechanics
convert that answer into reordering.

Equivalently: fair scheduling = FAIR's throttler with **deferral instead of
rejection**. A strictly better trade — a deferred job isn't lost capacity,
just later.

## Core loop: probabilistic bounce at dequeue

```
on dequeue of job j from tenant k:
    score := sketch.Query(k)
    if rand() < bounceProbability(score) and j.bounces < maxBounces and queueNotEmpty:
        defer j                       // requeue with delay ∝ score (preferred)
                                      // or push to back of queue
    else:
        execute j
        sketch.Update(k, +cost)       // cost = runtime seconds, rows, tokens…
```

- Heavy tenants' jobs are bounced probabilistically; light tenants' jobs
  sail through the *same* queue. Interleaving converges toward fair sharing
  stochastically — SFB-style, not exact WFQ.
- `maxBounces` + "serve anyway if nothing else waits" makes it
  **work-conserving**: a queue holding only heavy-tenant jobs runs at full
  speed. Fairness engages only under actual competition — FAIR's "only
  throttle under genuine shortage" philosophy, transplanted.
- **Delay-based deferral beats back-of-queue bouncing**: one queue
  round-trip instead of many, and it preserves per-tenant FIFO order
  (a bounced job never lands behind its own tenant's later jobs).

## Penalty policy: two interchangeable modes

Both are policies over the same sketch; pick per deployment.

### Mode 1: Backlog-triggered (BLUE-style) — no configuration, default

The naive approach needs each tenant's fair share, which requires the active
tenant count — unknowable from a sketch. Sidestep: **penalize only on
congestion evidence**, using job age at dequeue as the signal:

```
on dequeue:
    if j.age > latencySLO:            // queue is congested
        sketch.Update(k, +Pi)         // BLUE-style increment
on successful execute (age within SLO):
    sketch.Update(k, −Pd)             // or rely on decay alone
```

Heavy tenants appear at the dequeue point more often, so under congestion
they absorb penalty *in proportion to their contribution to the backlog* —
no shares, no tenant count, no fair-share arithmetic. When the queue is
fast, nobody accrues penalty and the scheduler is invisible. This is BLUE's
queue-overflow increment reincarnated, and it is the same self-tuning trick
FAIR already validates. Decay: `Exponential(λ)`, as in FAIR.

### Mode 2: Explicit shares (debt meter)

When you *do* want configured rates (tenant tiers): score = consumed
service, `Linear(r)` drain where r is the tenant's entitled service rate,
`bounceProbability` keyed off debt above a burst allowance. This is
[ratelimiter.md](ratelimiter.md)'s debt meter applied at dequeue-time with
deferral instead of rejection. Tenant weights fall out as
`Update(k, cost/weight)` in either mode.

### bounceProbability

v1: identity on the score clamped to `[0, pMax]` with `pMax < 1` (never
starve; combined with maxBounces this bounds worst-case added latency to
roughly `maxBounces × redeliveryDelay`). Shape refinements (convex ramps,
hysteresis) are eval questions, not design questions.

## Integration adapters (the deployability story)

The mechanism is "judge at dequeue, maybe defer" — which retrofits onto
queues we don't control:

| Queue | Defer mechanism | Notes |
|---|---|---|
| **SQS** | Don't delete; `ChangeMessageVisibility(delay ∝ score)` | Deferral is native; bounce count from `ApproximateReceiveCount`; mind `maxReceiveCount`/DLQ interaction |
| **Redis (BullMQ/Sidekiq-style)** | Move to delayed set with `not_before = now + delay` | Or `LMOVE` to tail (loses per-tenant order; not preferred) |
| **Postgres SKIP LOCKED** | `UPDATE jobs SET not_before = now() + delay` | One-statement defer; index on `(not_before, id)` |
| **In-process (Go channel / worker pool)** | Timer wheel or delayed heap in the shim | Reference implementation lives here |
| **Kafka** | ❌ out of scope | Can't skip within a partition without client-side buffering |

The shim needs three integration points: a dequeue hook, a defer verb, and a
cost signal at completion. Everything else is internal.

## API sketch (Go)

```go
package schedule // import "github.com/satmihir/grudge/schedule"

type Config struct {
    // Mode 1 (default): backlog-triggered. LatencySLO is the congestion
    // signal; Pi/Pd as in FAIR.
    LatencySLO time.Duration

    // Mode 2: explicit shares. Setting Rate > 0 switches modes.
    Rate  float64 // entitled tokens/sec per unit weight
    Burst float64

    MaxBounces uint32  // work-conservation cap (default 3)
    PMax       float64 // bounce probability ceiling (default 0.95)

    // Sketch sizing; zero values → tune.SuggestDimensions defaults.
    Levels        uint32
    CellsPerLevel uint32
    RotationPeriod time.Duration
}

type Scheduler struct { /* wraps a grudge rotate.Rotator */ }

func New(cfg Config) (*Scheduler, error)

// Decision is called at dequeue. age is the job's time-in-queue; bounces
// its redelivery count; queueDepth lets the scheduler skip deferral when
// nothing else is waiting (work conservation).
type Verdict struct {
    Defer bool
    Delay time.Duration // suggested not-before offset when Defer
}
func (s *Scheduler) Decide(tenant []byte, weight float64, age time.Duration,
    bounces uint32, queueDepth int) Verdict

// Report is called at job completion with the resource cost consumed.
func (s *Scheduler) Report(tenant []byte, weight float64, cost float64)

func (s *Scheduler) Close()
```

Adapters (`schedule/sqs`, `schedule/redis`, `schedule/pgx`) wrap Decide/
Report around each queue's receive loop.

## Guarantees and non-guarantees

**Guaranteed (by construction):**
- Constant memory: L×M×16B regardless of tenant count.
- Collision shielding: a light tenant is deferred beyond its share only if
  all L of its cells collide with heavy tenants — `(1−(1−1/M)^H)^L`, same
  math and tuning helper as FAIR.
- Transience: rotation bounds any false-positive penalty to
  `Generations × Period`.
- Work conservation: capacity is never idled by fairness (maxBounces +
  queue-empty check).
- Bounded added latency for any single job: ~`maxBounces × redeliveryDelay`.

**Not guaranteed (the fidelity traded for constant memory):**
- Exact max-min fairness — allocation converges stochastically; per-window
  Jain scores will trail true WFQ.
- Smooth tail latency for heavy tenants — bounce position is randomized,
  not deadline-ordered; expect noisier p99 than DRR/WFQ for the over-share
  tenant.
- Strict global ordering — cross-tenant reordering is the entire point.

**Costs:**
- Bounce churn: each deferral is a queue round-trip; under heavy unfairness
  the requeue traffic is roughly proportional to the reordering demanded.
  This is the metric to watch in the eval, and the reason delay-based
  deferral is the default.

## Evaluation plan

`cmd/fairness_eval` (Jain index vs. max-min entitlement, utilization
guardrail) is exactly the right instrument — port the scenario from
"throttle" to "defer":

1. **Noisy neighbor**: 18 well-behaved + 2 flooding tenants sharing a
   worker pool; measure per-tenant completion rates vs. entitlement, Jain
   index, p50/p99 wait by tenant class — against (a) plain FIFO,
   (b) per-tenant queues + round-robin (the memory-expensive gold
   standard), (c) this scheduler.
2. **Bounce budget sweep**: Jain and churn vs. `maxBounces` ∈ {1..8} — find
   the knee.
3. **Open research question**: how close does bounce-based stochastic
   reordering get to max-min fairness as a function of bounce budget and
   sketch error? Nobody has published this; the eval harness can answer it
   empirically, and it is likely a paper if the answer is clean.
4. **Churn accounting**: requeue round-trips per executed job vs. degree of
   unfairness — the honest cost curve for the README.

## Open questions

1. **Delay function shape**: `delay ∝ score` is v1; should it instead
   target the tenant's virtual eligibility time (score/r) for
   Mode 2 — closer to earliest-eligible-virtual-deadline semantics?
2. **Multi-worker consistency**: N workers dequeue concurrently, each
   consulting the shim. Sketch updates are safe (grudge handles
   concurrency), but do workers share one sketch (a small local service) or
   run one each (fairness converges anyway, slower)? Leaning: one per
   process, document convergence behavior; the mergeability law offers a
   gossip path if it matters.
3. **Priority × fairness composition**: most real queues have priority
   tiers. Fairness within tier is trivial (one scheduler per tier); should
   anything smarter cross tiers?
4. **Starvation pathology check**: a tenant whose jobs are always deferred
   `maxBounces` times still executes — but verify the eval shows no
   metastable state where bounced jobs pile up and age-trigger penalties on
   innocents (age > SLO increments the *tenant of the aged job*, who may be
   a victim, not a cause — does Mode 1 need victim/cause discrimination,
   e.g., only increment when the dequeued job is *executed* late rather
   than on every late observation?).
5. **Naming**: subpackage of grudge, or its own product name alongside the
   rate limiter?

## Prior art map

- **SFQ** (McKenney): hash flows → fixed lanes, round-robin, perturb. No
  multi-level shielding; requires owning the queue.
- **DRR / WFQ**: exact, per-flow state, owns the queue.
- **BLUE / SFB**: congestion-triggered probabilistic penalties — Mode 1's
  direct ancestor, via FAIR.
- **Temporal fairness keys, shuffle sharding, per-tenant queues**: the
  state-heavy incumbents this displaces.
- **Gap**: nothing in the Sidekiq/BullMQ/SQS/Postgres-queue ecosystem
  offers drop-in fairness without queue topology changes. This is arguably
  a stronger original contribution than the rate limiter (which has GCRA as
  prior art).
