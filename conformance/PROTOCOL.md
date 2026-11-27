# FAIR tracker conformance v1: rotation pilot

Implement a tracker library and a thin `fair-conformance` executable. One invocation starts one fresh tracker. Read one JSON object per line from standard input, in order; write one JSON object per operation on standard output in the same order. Exit successfully on EOF. No banners or logs on stdout. Only these valid inputs are in scope; no arguments, network, credentials, or filesystem state are required.

Use three levels, 64 buckets per level, failure increment 0.25, success decrement 0.125, decay rate 0.1, statistics enabled, and minimum final probability. Time is fixed (no decay or automatic ticks). Client IDs are nonempty UTF-8 strings encoded as bytes. All operations are sequential. Match the pinned Go tracker's behavior, including clamping to [0,1] and updating the standby structure. Do not expose or compare random draws.

| Input | Output |
| --- | --- |
| `{"op":"register","client":"A"}` | `{"probability":P}`: the active tracker's numeric final throttling probability, absolute tolerance 1e-12 |
| `{"op":"failure","client":"A"}` / `{"op":"success","client":"A"}` | `{}`: apply the outcome for that client |
| `{"op":"rotate"}` | `{}`: promote the warm standby to active and create a fresh standby; do not return until the rotation takes effect |

Public smoke example: `register A`, `failure A`, `register A`, `rotate`, `register A` must yield probabilities `0`, `0.25`, `0.25` on the three registrations. A second rotation with no intervening outcomes promotes a cold standby. This version tests a slice of tracker behavior, **not** full library equivalence, performance, or randomness.
