package config

import "time"

// The config for the underlying data structure. Largely for internal use.
type FairnessTrackerConfig struct {
	// Size of the row at each level
	M uint32
	// Number of levels in the structure
	L uint32
	// The delta P to add to a bucket's probability when there's an error
	Pi float64
	// The delta P to subtract from a bucket's probability when there's a success
	Pd float64
	// Maximum grace tokens a bucket can accumulate
	GraceTokenLimit uint32
	// The exponential decay rate for the probabilities
	Lambda float64
	// The frequency of rotation
	RotationFrequency time.Duration
	// Include result stats. Useful for debugging but may slightly affect performance.
	IncludeStats bool
	// The function to choose the final probability from all the bucket probabilities
	FinalProbabilityFunction FinalProbabilityFunction
}
