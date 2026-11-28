package config

import (
	"fmt"
	"math"
	"time"

	"github.com/satmihir/fair/pkg/logger"
)

const (
	// Number of concurrent clients you expect to your app if not user-provided
	defaultExpectedClientFlows = 1000
	// Number of buckets per level
	defaultBucketsPerLevel = 1000
	// Number of acceptable "bad" requests before a flow gets fully shut down
	defaultTolerableBadRequestsPerBadFlow = 25
	// The decay rate lambda of the probability with time to avoid permanently banning workloads
	defaultDecayRate = 0.01
	// Percent of the expected client flows assumed to be "bad" in the sense of
	// needing fairness throttle (.1%)
	percentBadClientFlows = 0.001
	// The "low" probability to target when deciding various parameters
	lowProbability = 0.0001
	// The slowing factor from Pi to Pd (10x successes to get a flow fully exonerated)
	pdSlowingFactor = 0.001
	// The minimum number of levels to use despite what the calculation says
	minL = 3
	// The default rotation duration
	defaultRotationDuration = time.Minute * 5
)

// FinalProbabilityFunction chooses a final probability from a slice of bucket
// probabilities.
type FinalProbabilityFunction func([]float64) float64

var (
	generateTunedStructureConfig = GenerateTunedStructureConfig

	// MinFinalProbabilityFunction returns the smallest probability in the
	// slice. It is the default implementation used by the tracker.
	MinFinalProbabilityFunction FinalProbabilityFunction = func(buckets []float64) float64 {
		if len(buckets) == 0 {
			logger.Fatalf("Cannot compute final probability with empty buckets slice")
		}

		minVal := 1.
		for _, b := range buckets {
			minVal = math.Min(minVal, b)
		}

		return minVal
	}

	// MeanFinalProbabilityFunction returns the mean of all bucket
	// probabilities and can be used in scenarios where the minimum value is
	// too strict.
	MeanFinalProbabilityFunction FinalProbabilityFunction = func(buckets []float64) float64 {
		if len(buckets) == 0 {
			logger.Fatalf("Cannot compute final probability with empty buckets slice")
		}

		var total float64
		for _, b := range buckets {
			total += b
		}

		return total / float64(len(buckets))
	}
)

// DefaultFairnessTrackerConfig returns a configuration that should work well
// for most applications without any additional tuning.
func DefaultFairnessTrackerConfig() *FairnessTrackerConfig {
	conf, err := generateTunedStructureConfig(
		defaultExpectedClientFlows,
		defaultBucketsPerLevel,
		defaultTolerableBadRequestsPerBadFlow)
	if err != nil {
		// This should never happen with valid defaults
		logger.Fatalf("failed to generate default config: %v", err)
	}
	return conf
}

// GenerateTunedStructureConfig creates a configuration tuned for the expected
// scale of your application.
//
// Parameters:
//   - expectedClientFlows: number of concurrent clients you expect.
//   - bucketsPerLevel: number of buckets per level in the data structure.
//   - tolerableBadRequestsPerBadFlow: number of failed requests tolerated before
//     a flow is fully blocked.
//
// All arguments must be positive. At least two buckets per level are needed
// to avoid certain collisions. An unrepresentable level count is an error.
func GenerateTunedStructureConfig(expectedClientFlows, bucketsPerLevel, tolerableBadRequestsPerBadFlow uint32) (*FairnessTrackerConfig, error) {
	if expectedClientFlows == 0 {
		return nil, fmt.Errorf("expectedClientFlows must be greater than 0")
	}
	if bucketsPerLevel < 2 {
		return nil, fmt.Errorf("bucketsPerLevel must be at least 2")
	}
	if tolerableBadRequestsPerBadFlow == 0 {
		return nil, fmt.Errorf("tolerableBadRequestsPerBadFlow must be greater than 0")
	}

	M := uint32(math.Ceil(float64(expectedClientFlows) * percentBadClientFlows))
	L := CalculateL(bucketsPerLevel, M, lowProbability)
	if L == 0 {
		return nil, fmt.Errorf("required number of levels is not representable for %d bad flows and %d buckets per level", M, bucketsPerLevel)
	}

	// The probability to add per bad outcome so we fully block a flow after tolerable failures
	Pi := 1 / float64(tolerableBadRequestsPerBadFlow)
	// We want a slower recovery than the speed of marking workloads as bad
	Pd := pdSlowingFactor * Pi

	c := &FairnessTrackerConfig{
		M:                        bucketsPerLevel,
		L:                        max(L, minL),
		Pi:                       Pi,
		Pd:                       Pd,
		Lambda:                   defaultDecayRate,
		RotationFrequency:        defaultRotationDuration,
		IncludeStats:             false,
		FinalProbabilityFunction: MinFinalProbabilityFunction,
	}
	if err := c.ValidateStructure(); err != nil {
		return nil, err
	}
	return c, nil
}

// CalculateL computes the levels needed for a collision probability at most p
// with B buckets per level and M bad flows: (1 - (1 - 1/B)^M)^L <= p.
// It returns 0 if B < 2, M == 0, p is not in (0, 1), or the required level
// count cannot be represented by a uint32.
func CalculateL(B, M uint32, p float64) uint32 {
	if B < 2 || M == 0 || !(p > 0 && p < 1) {
		return 0
	}

	// Compute log(1 - exp(x)) without losing the smaller probability to
	// cancellation when either collisions or empty buckets are rare.
	x := float64(M) * math.Log1p(-1/float64(B))
	var logCollision float64
	if x > -math.Ln2 {
		logCollision = math.Log(-math.Expm1(x))
	} else {
		logCollision = math.Log1p(-math.Exp(x))
	}
	if logCollision == 0 {
		return 0
	}
	levels := math.Ceil(math.Log(p) / logCollision)
	if !(levels >= 1 && levels <= math.MaxUint32) {
		return 0
	}
	return uint32(levels)
}
