# Fairness regression gate

From the repository root:

```sh
go test ./cmd/fairness_eval -run '^TestFairnessGate$' -count=1 -timeout=30s -v
```

A failing seed fails the command. This tests the default FAIR variant, not the evaluator's configuration sweep. Each workload runs for 120 simulated seconds at 40 tokens/second, starting with 40 tokens. The tracker rotates at 25, 50, 75, and 100 seconds; the simulation waits for each rotation before processing the next request. Seeds 17, 53, and 101 each drive Poisson arrivals and tracker decisions. Each trace is regenerated and replayed from scratch; per-client counts and the entire result must match exactly. Keep these tests serial because tracker decisions use the package-global RNG.

| Workload | Worst client entitlement ratio | Entitlement Jain index | Utilization |
| --- | ---: | ---: | ---: |
| 20 equal clients, 4 requests/second each | ≥ 0.70 | ≥ 0.98 | ≥ 0.98 |
| 16 steady clients at 2 requests/second plus 4 noisy clients at 20 | ≥ 0.50 | ≥ 0.75 | ≥ 0.98 |

Each floor applies to **every seed**, not the mean. Entitlement is the evaluator's max-min allocation of available capacity capped by realized client demand. The worst ratio protects a single underserved client that Jain's index could hide; Jain checks balance across all clients. Equal senders should approach parity, with allowance for arrival and throttle randomness. Tenfold rate skew is harder: the gate still requires every client to receive half its entitlement and limits overall imbalance. Across the three calibration seeds the minimum (worst ratio, Jain) was (0.756, 0.989) for equal clients and (0.550, 0.762) for noisy neighbors. Both workloads offer more than enough traffic to saturate the resource, so no more than 2% of capacity should be wasted. These are regression budgets for fixed workloads, not production guarantees.
