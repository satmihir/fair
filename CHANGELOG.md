# Changelog

## v2.0.0 (unreleased)

Version 2 hardens the library for production use. It changes the module path, a few function signatures, and some behavior, so it is a new major version.

### Upgrading from v1

1. Change imports from `github.com/satmihir/fair/pkg/...` to `github.com/satmihir/fair/v2/pkg/...`, and run `go get github.com/satmihir/fair/v2/pkg/tracker`.
2. Use Go 1.26 or newer.
3. Handle the new error from `config.GenerateTunedStructureConfig`, which now returns `(*FairnessTrackerConfig, error)`.
4. Replace the removed builder methods:
   - `builder.BuildWithDefaultConfig()` becomes `builder.Build()` on a fresh builder.
   - `builder.BuildWithConfig(cfg)` becomes `tracker.NewFairnessTracker(cfg)`.
5. If you construct `data.Structure` directly, drop the `includeStats` argument from `data.NewStructure` and `data.NewStructureWithClock` and set `IncludeStats` on the config instead.
6. If a simulation or test seeded `math/rand` with `rand.Seed` to make the tracker reproducible, set `HashKeySource` and `DecisionSource` on the config instead. `rand.Seed` has had no effect since Go 1.24.

### Breaking changes

- The module path is `github.com/satmihir/fair/v2`.
- The minimum Go version is 1.26.
- `config.GenerateTunedStructureConfig` returns an error for arguments it cannot tune, instead of a config that fails later.
- `FairnessTrackerBuilder.BuildWithDefaultConfig` and `BuildWithConfig` are removed. Both ignored the settings accumulated on the builder.
- `data.NewStructure` and `data.NewStructureWithClock` no longer take `includeStats`; it comes from the config.
- Configurations are validated more strictly at construction. A tracker rejects zero dimensions, increments outside `0 < Pd < Pi <= 1`, a negative or non-finite `Lambda`, a nil probability function, a non-positive rotation interval, and dimensions whose estimated storage exceeds `config.MaxRotationBytes` (64 MiB). A config that v1 accepted may now return an error.
- A tracker copies its config when it is built. Changing the config afterwards no longer affects a running tracker.
- The text of three configuration errors changed.

### Behavior changes

- **Bucket placement.** Clients are placed with SipHash-2-4 under a secret 128-bit key drawn from `crypto/rand` for every generation, replacing MurmurHash3 with a 32-bit seed. A client that chooses its own identifier can no longer aim it at another client's buckets. Each level's bucket is derived separately from the hash, so two clients share every level about as often as chance allows instead of at a rate near `1/M^2`.
- **Decay and the clock.** Decay measures elapsed time with the monotonic clock, so a wall-clock step no longer wipes penalties, and time never moves backward within a tracker.
- **Throttling decision.** A request is throttled when the random draw is below its probability, not at or below it, so a client at probability 0 is never throttled.
- **Outcomes.** `ReportOutcome` ignores a value other than `OutcomeSuccess` or `OutcomeFailure`. It used to count as a failure.
- **Custom probability functions.** The result is clamped to `[0, 1]`, and NaN falls back to the smallest bucket probability. NaN used to disable throttling.
- **Rotation.** A failed rotation keeps the current structures serving and is retried on the next tick; in v1 it ended the process with `log.Fatalf`. `Close` waits for the rotation worker and is safe to call more than once.
- **No process exit.** The library never ends the process. In v1 the built-in probability functions called `log.Fatalf` on an empty slice; they now return 0. `DefaultFairnessTrackerConfig` panics only if the library's own defaults are invalid.
- **Retired structures.** Rotation closes the structure it retires once no request can be using it.
- **Request cost.** `RegisterRequest` reads only the active structure, and neither it nor `ReportOutcome` allocates a hasher per call.

### Added

- `FairnessTracker.Counters` reports cumulative requests, throttles, outcomes and ignored outcomes.
- `FairnessTracker.Saturation` reports how much of the active structure is carrying penalties.
- `FairnessTracker.RotationStatus` reports rotation attempts and the last error.
- `config.FairnessTrackerConfig.HashKeySource` and `DecisionSource`, with matching builder setters, make simulations reproducible. Leave both nil in production.
- `config.FairnessTrackerConfig.ValidateStructure` and `config.MaxRotationBytes`.
- The `logger` package: logging is off by default and `logger.SetLogger` turns it on. Its `Logger` interface has three print methods and no fatal method.

### Removed

- `pkg/testutils` is no longer public. It holds helpers for this module's own tests and now lives under `internal/`.
- The module root is no longer a `main` package. The lint runner that lived there is `cmd/tasks`.
- The protobuf serialization package is no longer public. It was never part of a tagged release, is not wired to the tracker, and now lives under `internal/`.
- The dependency on `github.com/spaolacci/murmur3`.

### Documentation

- The README documents how long a penalty lasts: a client that stays fully throttled has its own failures discarded at the second successful rotation after the last one it reported, whatever `Lambda` is.
