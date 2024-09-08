package tuning

import (
	"math"

	"github.com/satmihir/fair/pkg/data"
)

const (
	// Number of concurrent clients you expect to your app if not user-provided
	defaultExpectedClientFlows = 1000
	// Number of buckets per level
	defaultBucketsPerLevel = 1000
	// Number of acceptable "bad" requests before a flow gets fully shut down
	defaultTolerableBadRequestsPerBadFlow = 25
	// Percent of the expected client flows assumed to be "bad" in the sense of
	// needing fairness throttle (.1%)
	percentBadClientFlows = 0.001
	// The "low" probability to target when deciding various parameters
	lowProbability = 0.0001
	// The slowing factor from Pi to Pd (10x successes to get a flow fully exonerated)
	pdSlowingFactor = 0.1
	// The minimum number og levels to use despite what the calculation says
	minL = 3
)

// The default config that's supposed to work in most cases
func DefaultStructureConfig() *data.StructureConfig {
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
func GenerateTunedStructureConfig(expectedClientFlows, bucketsPerLevel, tolerableBadRequestsPerBadFlow uint32) *data.StructureConfig {
	M := uint32(math.Ceil(float64(expectedClientFlows) * percentBadClientFlows))
	L := CalculateL(bucketsPerLevel, M, lowProbability)
	if L < minL {
		L = minL
	}

	// The probability to add per bad outcome so we fully block a flow after tolerable failures
	var Pi float64 = 1 / float64(tolerableBadRequestsPerBadFlow)
	// We want a slower recovery than the speed of marking workloads as bad
	Pd := pdSlowingFactor * Pi

	return &data.StructureConfig{
		M:  bucketsPerLevel,
		L:  L,
		Pi: Pi,
		Pd: Pd,
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
// Most users should use GenerateTunedStructureConfig which uses this function but it's
// kept public in case someone wants to do their own tuning.
func CalculateL(B, M uint32, p float64) uint32 {
	term := 1 - math.Pow(1-1/float64(B), float64(M))
	L := math.Log(p) / math.Log(term)
	return uint32(math.Ceil(L))
}
