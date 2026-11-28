package config

import (
	"fmt"
	"io"
	"math"
	"time"
)

// FairnessTrackerConfig defines the parameters for the underlying data
// structure used by the fairness tracker. Most users will rely on
// GenerateTunedStructureConfig to populate this struct.
type FairnessTrackerConfig struct {
	// Size of the row at each level
	M uint32
	// Number of levels in the structure
	L uint32
	// The delta P to add to a bucket's probability when there's an error
	Pi float64
	// The delta P to subtract from a bucket's probability when there's a success
	Pd float64
	// The exponential decay rate for the probabilities, per second. Zero
	// disables decay but not forgiveness: rotation still ends a penalty (see
	// RotationFrequency).
	Lambda float64
	// The interval between rotations. It also bounds how long a client's own
	// feedback is remembered: a client that stays fully throttled reports no
	// outcomes, so its failures are discarded at the second successful
	// rotation after the last one it reported, whatever Lambda is. That is
	// nominally one to two intervals. It is not a deadline for admission: a
	// failed rotation delays it, and buckets the client shares with others
	// can still be penalized. See "How long a penalty lasts" in the README.
	RotationFrequency time.Duration
	// Include result stats. Useful for debugging but may slightly affect performance.
	IncludeStats bool
	// The function to choose the final probability from all the bucket probabilities
	FinalProbabilityFunction FinalProbabilityFunction
	// HashKeySource supplies the secret keys that place clients in buckets.
	// Sixteen bytes are read for every structure a tracker builds, including
	// one at each rotation. Leave it nil in production so that keys come from
	// crypto/rand. A simulation or test can supply a seeded reader to make
	// placement reproducible; it has no effect on the random draw behind each
	// throttling decision. A reader shared by several trackers must be safe
	// for concurrent use.
	HashKeySource io.Reader
	// DecisionSource returns the uniform random number in [0, 1) behind each
	// throttling decision: a request is throttled when the number is below
	// the client's probability, so probability 0 never throttles. Leave it
	// nil in production to use math/rand's generator. A simulation or test
	// can supply a seeded source to make decisions reproducible; calling
	// rand.Seed no longer does that, because it has had no effect since
	// Go 1.24. The function must be safe for concurrent use if requests are
	// made concurrently.
	DecisionSource func() float64
}

// ValidateStructure checks the parameters needed by the underlying data structure.
// Dimensions must fit the rotating tracker's allocation budget. RotationFrequency
// is checked separately by callers that start a real ticker.
func (c *FairnessTrackerConfig) ValidateStructure() error {
	switch {
	case c == nil:
		return fmt.Errorf("FairnessTrackerConfig cannot be nil")
	case c.M == 0 || c.L == 0:
		return fmt.Errorf("dimensions M and L must be positive, got M=%d L=%d", c.M, c.L)
	case !(0 < c.Pd && c.Pd < c.Pi && c.Pi <= 1):
		return fmt.Errorf("probability increments must satisfy 0 < Pd < Pi <= 1, got Pd=%g Pi=%g", c.Pd, c.Pi)
	case math.IsNaN(c.Lambda) || math.IsInf(c.Lambda, 0) || c.Lambda < 0:
		return fmt.Errorf("decay rate Lambda must be finite and nonnegative, got %g", c.Lambda)
	case c.FinalProbabilityFunction == nil:
		return fmt.Errorf("FinalProbabilityFunction cannot be nil")
	}
	return checkRotationBudget(c.L, c.M)
}
