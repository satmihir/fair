package config

import "time"

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
}
