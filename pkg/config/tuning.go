package config

import (
	"log"
	"math"
	"time"
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
	// MinFinalProbabilityFunction returns the smallest probability in the
	// slice. It is the default implementation used by the tracker.
	MinFinalProbabilityFunction FinalProbabilityFunction = func(buckets []float64) float64 {
		if len(buckets) == 0 {
			log.Fatalf("Cannot compute final probability with empty buckets slice")
		}

		var minVal float64 = 1.
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
			log.Fatalf("Cannot compute final probability with empty buckets slice")
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
	return GenerateTunedStructureConfig(
		defaultExpectedClientFlows,
		defaultBucketsPerLevel,
		defaultTolerableBadRequestsPerBadFlow)
}

// Generates a "good enough" config to use for a structure underneath the throttler
// which requires minimal tuning and should be able to get decent results in most
// cases. If more tuning is desired, the clients can directly provide their own
// config object when initializing FairWorkloadTracker.
//
// params:
// -------
// expectedClientFlows - Number of concurrent clients you expect to your app
// bucketsPerLevel - Number of buckets per level in the core structure
// tolerableBadRequestsPerBadFlow - Number of requests we can tolerate before we fully shut down a flow
// GenerateTunedStructureConfig creates a configuration tuned for the expected
// scale of your application.
//
// Parameters:
//   - expectedClientFlows: number of concurrent clients you expect.
//   - bucketsPerLevel: number of buckets per level in the data structure.
//   - tolerableBadRequestsPerBadFlow: number of failed requests tolerated before
//     a flow is fully blocked.
func GenerateTunedStructureConfig(expectedClientFlows, bucketsPerLevel, tolerableBadRequestsPerBadFlow uint32) *FairnessTrackerConfig {
	// If caller passes 0, fall back to the sane default and warn.
    if tolerableBadRequestsPerBadFlow == 0 {
        log.Printf("tolerableBadRequestsPerBadFlow=0 is invalid; falling back to default %d", defaultTolerableBadRequestsPerBadFlow)
        tolerableBadRequestsPerBadFlow = defaultTolerableBadRequestsPerBadFlow
    }
	
	M := uint32(math.Ceil(float64(expectedClientFlows) * percentBadClientFlows))
	L := CalculateL(bucketsPerLevel, M, lowProbability)
	if L < minL {
		L = minL
	}

	// The probability to add per bad outcome so we fully block a flow after tolerable failures
	Pi := 1 / float64(tolerableBadRequestsPerBadFlow)
	// We want a slower recovery than the speed of marking workloads as bad
	Pd := pdSlowingFactor * Pi

	return &FairnessTrackerConfig{
		M:                        bucketsPerLevel,
		L:                        L,
		Pi:                       Pi,
		Pd:                       Pd,
		Lambda:                   defaultDecayRate,
		RotationFrequency:        defaultRotationDuration,
		IncludeStats:             false,
		FinalProbabilityFunction: MinFinalProbabilityFunction,
	}
}

// Get the appropriate number of levels to achieve the target collision probability:
//
// params:
// -------
// B - Buckets per level.
// M - Expected "bad" client flows that'll need throttling.
// p - Probability of an innocent flow colliding with a bad one. Likely a small value.
//
// The formula we are solving with:
// p = (1 - (1 - (1/B))^M)^L
// Comes from the following paper:
// https://rtcl.eecs.umich.edu/rtclweb/assets/publications/2001/feng2001fair.pdf
//
// CalculateL computes the number of levels required to achieve the target
// collision probability. Most users should call GenerateTunedStructureConfig
// instead of invoking this directly.
func CalculateL(B, M uint32, p float64) uint32 {
	term := 1 - math.Pow(1-1/float64(B), float64(M))
	L := math.Log(p) / math.Log(term)
	return uint32(math.Ceil(L))
}
