# Test Plan: pkg/config

## Target: MinFinalProbabilityFunction
- **Type:** var FinalProbabilityFunction (func([]float64) float64)
- **Dependencies:** pure function; no external I/O or clocks
- **Contracts:**
  - Returns the minimum value from a non-empty slice.
  - Input slice must be non-empty; empty slice currently terminates process via fatal log (design issue for libraries).

### Example Cases
1. **single element**
   - Arrange: buckets = [0.42]
   - Act: call function
   - Assert: returns 0.42
2. **distinct values**
   - Arrange: buckets = [0.1, 0.7, 0.3]
   - Act: call function
   - Assert: returns 0.1
3. **duplicates**
   - Arrange: buckets = [0.6, 0.6, 0.9]
   - Act: call function
   - Assert: returns 0.6

### Negative/Error Cases
- Empty slice: current behavior is `os.Exit(1)` via fatal log. Recommend refactor to return `(float64, error)` or panic with a specific type for testability.

### Properties
- Idempotence: calling twice with same slice yields same result.
- Ordering invariance: result independent of input order.
- Range preservation: output ∈ [min(input), max(input)].

## Target: MeanFinalProbabilityFunction
- **Type:** var FinalProbabilityFunction (func([]float64) float64)
- **Dependencies:** pure function; no external I/O or clocks
- **Contracts:**
  - Returns arithmetic mean of a non-empty slice.
  - Input slice must be non-empty; empty slice currently terminates process via fatal log (design issue for libraries).

### Example Cases
1. **single element**
   - Arrange: buckets = [0.5]
   - Act: call function
   - Assert: returns 0.5 (ε=1e-12)
2. **distinct values**
   - Arrange: buckets = [0.1, 0.2, 0.3]
   - Act: call function
   - Assert: returns 0.2 (ε=1e-12)
3. **with duplicates**
   - Arrange: buckets = [0.25, 0.25, 0.75, 0.75]
   - Act: call function
   - Assert: returns 0.5 (ε=1e-12)

### Negative/Error Cases
- Empty slice: current behavior is `os.Exit(1)` via fatal log. Recommend refactor to return `(float64, error)` or panic with a specific type for testability.

### Properties
- Idempotence: calling twice with same slice yields same result.
- Ordering invariance: result independent of input order.
- Bounds: result ∈ [min(input), max(input)] for inputs in [0,1]; equals min when all entries equal min; equals max when all entries equal max.

## Target: GenerateTunedStructureConfig
- **Type:** func(expectedClientFlows, bucketsPerLevel, tolerableBadRequestsPerBadFlow uint32) *FairnessTrackerConfig
- **Dependencies:** pure math; uses defaults from package; no external I/O
- **Contracts:**
  - `M == bucketsPerLevel`.
  - `L == max(minL, ceil( ln(p) / ln(1 - (1 - 1/B)^M_bad) ))`, where `B=bucketsPerLevel`, `M_bad=ceil(expectedClientFlows * percentBadClientFlows)`, `p=lowProbability`.
  - `Pi == 1 / float64(tolerableBadRequestsPerBadFlow)`.
  - `Pd == pdSlowingFactor * Pi`.
  - `Lambda == defaultDecayRate`.
  - `RotationFrequency == defaultRotationDuration`.
  - `IncludeStats == false`.
  - `FinalProbabilityFunction == MinFinalProbabilityFunction` by default.

### Example Cases
1. **defaults-equivalent inputs**
   - Arrange: expected=1000, B=1000, tolerable=25
   - Act: call function
   - Assert: `M=1000`, `L=3` (since raw L≈1.333→ceil=2 then minL=3), `Pi=0.04`, `Pd=0.00004`, `Lambda=0.01`, `RotationFrequency=5m`, `IncludeStats=false`, `FinalProbabilityFunction==MinFinalProbabilityFunction` (ε=1e-12 for floats).
2. **higher expected flows increases raw L before clamp**
   - Arrange: expected=200_000, B=1000, tolerable=25
   - Act: call function
   - Assert: `L >= 3` and `L` not less than default case; verify with direct `CalculateL` computation for consistency.
3. **larger buckets reduce L**
   - Arrange: expected=50_000 (so M_bad>1), B1=256, B2=8192
   - Act: call function for each B
   - Assert: `L(B2) <= L(B1)` (subject to minL clamp).
4. **tolerable failures affects Pi/Pd**
   - Arrange: tolerable=25 and tolerable=100
   - Act: call function for each
   - Assert: `Pi(100) == 0.01`, `Pd(100) == 0.00001`; and `Pi(100) < Pi(25)`, `Pd(100) < Pd(25)`.

### Negative/Error Cases
- `tolerableBadRequestsPerBadFlow == 0` leads to division-by-zero producing `+Inf` Pi (undesirable). Recommend input validation returning `ErrConfigInvalid`.
- Extremely small/large inputs: ensure no NaNs/Infs propagate in returned struct; otherwise validate and error.

### Properties
- Monotonicity (subject to `minL` clamp):
  - Increasing `expectedClientFlows` (thus `M_bad`) should not decrease `L`.
  - Increasing `bucketsPerLevel` should not increase `L`.
  - Increasing `tolerableBadRequestsPerBadFlow` decreases `Pi` and `Pd` proportionally.
- Determinism: same inputs always produce identical config.

## Target: DefaultFairnessTrackerConfig
- **Type:** func() *FairnessTrackerConfig
- **Dependencies:** calls `GenerateTunedStructureConfig` with package defaults
- **Contracts:**
  - Equivalent to `GenerateTunedStructureConfig(defaultExpectedClientFlows, defaultBucketsPerLevel, defaultTolerableBadRequestsPerBadFlow)`.
  - With current defaults: `M=1000`, `L=3`, `Pi=0.04`, `Pd=0.00004`, `Lambda=0.01`, `RotationFrequency=5m`, `IncludeStats=false`, `FinalProbabilityFunction==MinFinalProbabilityFunction`.

### Example Cases
1. **matches derived expectations**
   - Arrange: none
   - Act: call function
   - Assert: all fields match the above (ε=1e-12 for floats).

### Negative/Error Cases
- None (no inputs). If underlying defaults change, test should fail with clear diff.

### Properties
- Idempotence: calling twice returns configs with identical field values.

## Target: CalculateL
- **Type:** func(B, M uint32, p float64) (uint32, error)
- **Dependencies:** pure math; no external I/O or clocks
- **Contracts:**
  - Computes `L = ceil( ln(p) / ln(1 - (1 - 1/B)^M) )` for valid inputs `B>=1`, `M>=1`, `0<p<1`.
  - Returns an error for invalid inputs (recommended; current implementation always returns `nil` error).

### Example Cases
1. **typical small M**
   - Arrange: B=1000, M=1, p=1e-4
   - Act: call function
   - Assert: returns `2` (ε applied to intermediate calc only; final is integer).
2. **larger M increases L**
   - Arrange: B=1000, M=10, p=1e-4
   - Act: call function
   - Assert: returns `2` or higher; and `L(M=10) >= L(M=1)`.
3. **larger B reduces L**
   - Arrange: M=100, p=1e-4, compare B=256 vs B=4096
   - Act: call function
   - Assert: `L(B=4096) <= L(B=256)`.

### Negative/Error Cases
- `B==0` → division by zero in formula: should return an error (currently unguarded).
- `M==0` → `term==0`, `ln(term)=-Inf`: should return an error (currently unguarded).
- `p<=0` or `p>=1` → invalid probability for log: should return an error (currently unguarded).

### Properties
- Monotonicity: for fixed `p`, `L` non-decreasing in `M` and non-increasing in `B`.
- `p` sensitivity: decreasing `p` (harder target) should not decrease `L`.
- Determinism: identical inputs produce identical outputs.

## Notes on Test Determinism and Style
- All tests will follow AAA structure and use `require` assertions.
- No real time, randomness, or external I/O is involved; tests are fully deterministic.
- For floating comparisons, use ε=1e-12 tolerance where applicable.
- We will not attempt to capture `os.Exit` from fatal logs in unit tests. Instead, we propose refactoring fatal paths to return errors for testability and better library ergonomics. Until refactor, empty-slice cases for the FinalProbabilityFunction variables will be excluded from unit tests and covered via design validation.


