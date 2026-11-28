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
	// The exponential decay rate for the probabilities
	Lambda float64
	// The frequency of rotation
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
}

// ValidateStructure checks the parameters needed by the underlying data structure.
// Dimensions must fit the rotating tracker's allocation budget. RotationFrequency
// is checked separately by callers that start a real ticker.
func (c *FairnessTrackerConfig) ValidateStructure() error {
	switch {
	case c == nil:
		return fmt.Errorf("FairnessTrackerConfig cannot be nil")
	case c.M == 0 || c.L == 0:
		return fmt.Errorf("M and L must be positive, got M=%d L=%d", c.M, c.L)
	case !(0 < c.Pd && c.Pd < c.Pi && c.Pi <= 1):
		return fmt.Errorf("probability increments must satisfy 0 < Pd < Pi <= 1, got Pd=%g Pi=%g", c.Pd, c.Pi)
	case math.IsNaN(c.Lambda) || math.IsInf(c.Lambda, 0) || c.Lambda < 0:
		return fmt.Errorf("Lambda must be finite and nonnegative, got %g", c.Lambda)
	case c.FinalProbabilityFunction == nil:
		return fmt.Errorf("FinalProbabilityFunction cannot be nil")
	}
	return checkRotationBudget(c.L, c.M)
}
