# Crazy Ideas: Algorithms That Fall Out of grudge

**Status:** Brainstorm dump — breadth over rigor. Each entry here is a
candidate for promotion to its own spec doc (as was done for
[ratelimiter.md](ratelimiter.md)).
**Author:** Mihir Sathe (with Claude Fable)
**Date:** 2026-07-05
**Depends on:** [grudge.md](grudge.md)

## The unifying insight: four readings of a cell

A grudge cell — `(score, lastUpdated)` with pluggable decay, additive
updates, clamping — supports four canonical *readings*. Each reading is a
different algorithm family. Everything below is one of these four with a
domain costume on.

| # | Reading | Update pattern | Decay | What score means |
|---|---------|----------------|-------|------------------|
| 1 | Decayed counter | `+1` per event | Exponential(λ) | recency-weighted frequency; `Query·λ ≈` event rate |
| 2 | Saturating latch | `+1`, clamp `Hi=1` | Exponential | recency of membership (a Bloom filter that forgets) |
| 3 | Feedback score | `±δ` on outcomes | Exponential | reputation / health / penalty (FAIR's home turf) |
| 4 | Debt meter | `+cost` per consumption | Linear(r) | budget consumed (token bucket, see ratelimiter.md) |

**The rate-estimator math (reading 1)**: a key receiving events at rate `ρ`
with decay `Exponential(λ)` reaches equilibrium score `≈ ρ/λ` (each event
adds 1; decay removes `λ·s` per second; steady state at `s = ρ/λ`). So
`Query(k)·λ` estimates a key's current event rate — over an unbounded key
space, in constant memory. Several ideas below are just this estimator plus
a policy.

---

## Reading 1: Decayed counter ideas

### 1a. Time-decayed heavy hitters ("decayed count-min sketch")

A CMS whose counters age continuously instead of via windowing. Hot-key
detection for caches and DB partitions, trending detection. Beats
sliding-window sketches (ECM-sketch et al.) that keep multiple snapshot
sketches — grudge gets the window for free via lazy decay. Caveat inherited
from the structure: no key enumeration; callers track top-k candidates
externally (heap of recently-queried keys) or use it as a filter
("is this key hot?") rather than a reporter ("which keys are hot?").

### 1b. Adaptive observability sampling — HIGH PRIORITY

Sample each log pattern / trace signature at a rate inversely proportional
to its estimated frequency: rare events always kept, floods automatically
downsampled. Per-signature state over unbounded signature spaces is exactly
why existing tail-samplers are memory-hungry; grudge does it in a few MB.
Sketch: `keep := rand() < targetRate / (Query(sig)·λ)`; then
`Update(sig, +1)`. Timely (observability cost crisis), clear buyer
(OpenTelemetry collector processors, log pipelines).

### 1c. Cache admission à la TinyLFU, with honest decay

TinyLFU (inside Caffeine, Ristretto) approximates aging by periodically
*halving all counters* — a coarse, stop-the-world stand-in for exactly the
exponential decay grudge computes lazily and continuously. A decay-native
admission filter (admit new key iff its decayed frequency exceeds the
eviction victim's) is a direct replacement with a sharper story. Existing,
identifiable community to pitch to.

### 1d. Temperature sketch for tiered storage

Graded hot/warm/cold scores over billions of object IDs to drive
SSD/HDD/archive migration. Real systems approximate this with per-object
LRU bits; grudge gives a continuous temperature in constant memory.

---

## Reading 2: Saturating latch ideas

### 2a. A Bloom filter that forgets (streaming dedup) — HIGH PRIORITY

Set membership with built-in TTL: `Update(k, +1)` clamped at `Hi=1` on
insert; membership = `Query(k) > threshold`. Answers "have I seen this
message/event/alert ID in the last N minutes?" over infinite streams with no
timer wheels, no eviction, no unbounded growth. Generalizes "stable Bloom
filters" from the literature with a principled, tunable TTL (threshold +
λ choose the effective window). Smallest spec of the bunch; zero new grudge
primitives needed; enormous audience (every at-least-once pipeline dedups).

### 2b. Novelty detection (the latch, inverted)

`Query(k) ≈ 0` means "never seen recently" — the security primitive behind
new-process-hash, first-contact-domain, and beaconing detection. Same
structure as 2a, security framing. Note the error direction flips vs 2a:
collisions make things look *familiar* (min-aggregation under-reports
novelty is false-NEGATIVE for alerts) — analyze before promising semantics.

---

## Reading 3: Feedback score ideas

### 3a. Generalized flap damping

**BGP route flap damping is literally this algorithm**: per-route penalty
accumulates on flap, decays exponentially with a half-life, suppress above a
threshold, reinstate below a lower threshold (hysteresis). Grudge is a flap
damper for anything: alert flap suppression, service-discovery instance
health, noisy-monitor squelching. Pleasing lineage: FAIR comes from SFB,
this from RFD — both networking-native uses of the same cell. Hysteresis
(two thresholds) is pure caller policy; needs nothing new from grudge.

### 3b. Circuit breakers over unbounded endpoint spaces

Per-dependency error scores where a per-key map won't fit: one score per
(host, route, tenant) triple, millions of combinations. Trip the breaker on
threshold, and decay *is* the half-open recovery policy — no state machine.

### 3c. Credential-stuffing / account-protection scoring

Decaying failure scores keyed by username AND by source IP (two sketches or
one with prefixed keys). Account lockout without state explosion, no
permanent lockout because decay is the forgiveness policy, and the
probabilistic structure means no clean enumeration oracle for attackers.

---

## Reading 4: Debt meter ideas (beyond ratelimiter.md)

### 4a. Retry budgets, per-key granular

Envoy-style retry budgets (retries may consume ≤ X% of request volume) but
per (dependency, tenant) at unbounded cardinality: debit on retry, linear
drain proportional to observed request rate.

### 4b. Distributed edge rate limiting — BIG PRIZE, phase 2

The mergeability law means edge nodes each run a local sketch and gossip
`DeltaSince` batches; convergence is guaranteed by the merge semantics
(up to clamp drift). This is the architecture Cloudflare-scale edge limiters
converge on ad hoc — grudge makes it principled, and it is exactly the
existing FAIR state-service design doc generalized. Belongs as a phase-2
section of ratelimiter.md rather than its own project.

---

## Speculative frontier (research-flavored)

### 5a. Non-stationary bandits over huge arm spaces

Two sketches: decayed reward sum and decayed pull count; their ratio is a
recency-weighted mean reward per arm, and decay handles non-stationarity
natively. Millions of arms (ad creatives, feed candidates, cache prefetch
choices) in constant memory. Open analysis problem: collision behavior of a
*ratio* of two min-aggregated estimates is not one-sided — needs real math
before promising anything. Publishable if it works.

### 5b. The physics footnote: LIF neurons

A grudge cell is a leaky integrate-and-fire neuron (exponential leak,
additive input, threshold firing). A sketch is a bank of millions of LIF
neurons indexed by hash. Probably a paper or a toy, not a product, but it
suggests event-correlation detectors are in the structure's natural
vocabulary.

---

## Known boundary: what does NOT fit

**Multiplicative updates break the merge law.** True AIMD (additive
increase, *multiplicative* decrease) and anything else with
state-proportional updates can't be expressed as additive deltas, so
`MergeDelta` convergence doesn't hold. Single-instance uses could still hack
it via read-modify-write, but it's outside grudge's contract. State this
crisply in grudge.md if any reading-3 idea drifts toward AIMD.

---

## Ranking for promotion to specs

1. **2a Forgetting Bloom filter / dedup** — smallest spec, huge audience,
   zero new primitives.
2. **1b Adaptive sampling** — timely, clear buyer, showcases the
   rate-estimator math.
3. **1c Decay-native TinyLFU** — sharp technical story, existing community
   (Ristretto/Caffeine users).
4. **4b Distributed merge rate limiting** — biggest prize but phase-2 of
   ratelimiter.md, not a new doc.

Implementation note for whoever picks these up: all of the above are policy
layers over the grudge API as spec'd in grudge.md (Sketch + Rotator +
Decay + TryUpdate). If an idea seems to need a new core primitive, that's a
signal to either redesign the idea or amend grudge.md first — keep the core
minimal.
