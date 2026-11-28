package data

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/spaolacci/murmur3"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/logger"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/utils"
)

// Represents a bucket in the leveled structure
type bucket struct {
	// Probability that a request falling on this bucket should be dropped
	probability float64
	// Latest time used for decay, in milliseconds since the structure's
	// epoch; never moves backward.
	lastUpdatedTimeMillis int64
	// Mutex protects the state of this bucket from concurrent access
	mu sync.Mutex
}

// Structure implements the Tracker interface using a multi-level Bloom filter
// style bucket layout. Each bucket tracks the probability that a request should
// be throttled based on the observed successes and failures for the hashed
// client identifier.
type Structure struct {
	// The data at all levels. Every value is a float64 representing the probability
	// of throttling the request.
	levels [][]*bucket
	// The config associated with this structure
	config config.FairnessTrackerConfig
	// The unique ID of the structure
	id uint64
	// The murmur hash seed
	murmurSeed uint32
	// The clock to use for getting the time
	clock utils.IClock
	// Construction time. Decay measures elapsed time from here with Time.Sub,
	// which uses the monotonic reading when the clock supplies one, so a
	// wall-clock step (NTP correction, manual change) is not mistaken for
	// elapsed time.
	epoch time.Time
	// Logs the first unusable final probability only.
	badProbability sync.Once
	// Includes stats in results. Useful for debugging but may slightly affect performance.
	includeStats bool
}

// NewStructureWithClock creates a Structure using the provided clock. This is
// primarily used in tests and simulations where time needs to be controlled.
func NewStructureWithClock(input *config.FairnessTrackerConfig, id uint64, includeStats bool, clock utils.IClock) (*Structure, error) {
	if input == nil {
		return nil, NewDataError(nil, "FairnessTrackerConfig cannot be nil")
	}
	cfg := *input
	if err := cfg.ValidateStructure(); err != nil {
		return nil, NewDataError(err, "invalid structure configuration")
	}
	if utils.IsNil(clock) {
		return nil, NewDataError(nil, "clock cannot be nil")
	}

	levels := make([][]*bucket, cfg.L)
	for i := range levels {
		levels[i] = make([]*bucket, cfg.M)

		for j := range levels[i] {
			levels[i][j] = &bucket{}
		}
	}

	return &Structure{
		levels:       levels,
		config:       cfg,
		id:           id,
		murmurSeed:   rand.Uint32(),
		clock:        clock,
		epoch:        clock.Now(),
		includeStats: includeStats,
	}, nil
}

// NewStructure creates a Structure using the real system clock.
func NewStructure(config *config.FairnessTrackerConfig, id uint64, includeStats bool) (*Structure, error) {
	return NewStructureWithClock(config, id, includeStats, utils.NewRealClock())
}

// GetID returns the identifier of this data structure.
func (s *Structure) GetID() uint64 {
	return s.id
}

// Close releases any resources associated with the Structure.
func (s *Structure) Close() {
}

// RegisterRequest records an incoming request from the client and returns the
// throttling decision based on current probabilities.
func (s *Structure) RegisterRequest(_ context.Context, clientIdentifier []byte) *request.RegisterRequestResult {
	var stats *request.ResultStats

	bucketProbabilities := make([]float64, s.config.L)

	// Taken while collecting, before caller-supplied code can touch the slice.
	minProbability := 1.

	// We can ignore the error since the handler never returns one
	s.visitBuckets(clientIdentifier, func(l uint32, m uint32, b *bucket) {
		bucketProbabilities[l] = b.probability
		minProbability = math.Min(minProbability, b.probability)
		if s.includeStats {
			if stats == nil {
				stats = &request.ResultStats{
					BucketIndexes: make([]int, s.config.L),
				}
			}
			stats.BucketIndexes[l] = int(m)
		}
	})

	pFinal := s.finalProbability(bucketProbabilities, minProbability)

	if s.includeStats {
		stats.BucketProbabilities = bucketProbabilities
		stats.FinalProbability = pFinal
	}

	// Decide whether to throttle the request based on the probability.
	shouldThrottle := rand.Float64() <= pFinal

	return &request.RegisterRequestResult{
		ShouldThrottle: shouldThrottle,
		ResultStats:    stats,
	}
}

// finalProbability keeps a custom FinalProbabilityFunction from silently
// switching throttling off or reporting an impossible probability: results
// outside [0, 1] are clamped, and NaN falls back to minProbability, the
// smallest bucket probability (what the default function returns). The
// caller computes that value itself because the function may modify the
// slice it is handed.
func (s *Structure) finalProbability(bucketProbabilities []float64, minProbability float64) float64 {
	p := s.config.FinalProbabilityFunction(bucketProbabilities)
	switch {
	case math.IsNaN(p):
		s.badProbability.Do(func() {
			logger.Printf("FinalProbabilityFunction returned NaN; using the minimum bucket probability instead (structure %d, further occurrences are not logged)", s.id)
		})
		return minProbability
	case p < 0:
		return 0
	case p > 1:
		return 1
	}
	return p
}

// ReportOutcome updates the probabilities for the buckets associated with the
// given client identifier based on the observed outcome. An outcome other than
// OutcomeSuccess or OutcomeFailure is ignored.
func (s *Structure) ReportOutcome(_ context.Context, clientIdentifier []byte, outcome request.Outcome) *request.ReportOutcomeResult {
	var adjustment float64
	switch outcome {
	case request.OutcomeSuccess:
		adjustment = -s.config.Pd
	case request.OutcomeFailure:
		adjustment = s.config.Pi
	default:
		return &request.ReportOutcomeResult{}
	}

	s.visitBuckets(clientIdentifier, func(_ uint32, _ uint32, b *bucket) {
		b.probability = math.Max(0, math.Min(1, b.probability+adjustment))
	})

	return &request.ReportOutcomeResult{}
}

// Visit the buckets belonging to the given clientIdentifier
// Also takes the bucket lock and manages probability decay prior to calling the handler
func (s *Structure) visitBuckets(clientIdentifier []byte, fn func(uint32, uint32, *bucket)) {
	levelHashes := generateNHashesUsing64Bit(clientIdentifier, s.config.L, s.murmurSeed)

	// One reading serves every level. A bucket that a concurrent visitor has
	// already advanced past it is left alone by the comparison below.
	// Sub saturates at about 292 years, so the subtraction cannot overflow.
	now := s.clock.Now().Sub(s.epoch).Milliseconds()

	for l := 0; l < int(s.config.L); l++ {
		lvl := s.levels[l]
		m := levelHashes[l] % s.config.M
		buck := lvl[m]

		buck.mu.Lock()

		if now > buck.lastUpdatedTimeMillis {
			elapsed := uint64(now - buck.lastUpdatedTimeMillis)
			buck.probability = adjustProbability(buck.probability, s.config.Lambda, elapsed)
			buck.lastUpdatedTimeMillis = now
		}

		fn(uint32(l), m, buck)
		buck.mu.Unlock()
	}
}

// Calculate n hashes of the given input using murmur hash.
// To optimize, we only calculate a single 64-bit hash and use a technique outlined in
// the paper below to compute more based on them:
// https://www.eecs.harvard.edu/~michaelm/postscripts/rsa2008.pdf
func generateNHashesUsing64Bit(input []byte, n uint32, seed uint32) []uint32 {
	// Compute the 64-bit hash
	h := murmur3.New64WithSeed(seed)
	h.Write(input)
	hash64 := h.Sum64()

	// Split the 64-bit hash into two 32-bit hashes
	hash1 := uint32(hash64)       // Lower 32 bits
	hash2 := uint32(hash64 >> 32) // Upper 32 bits

	// Generate the n hashes using the combination: hash_i = hash1 + i * hash2
	hashes := make([]uint32, n)
	for i := range hashes {
		hashes[i] = hash1 + uint32(i)*hash2
	}

	return hashes
}

// AdjustProbability applies exponential decay to the given probability.
// prob: the current probability value (between 0 and 1)
// lambda: the decay rate (higher values mean faster decay)
// deltaMs: the time difference in milliseconds
func adjustProbability(prob float64, lambda float64, deltaMs uint64) float64 {
	deltaSec := float64(deltaMs) / 1000.0
	decayedProb := prob * math.Exp(-lambda*deltaSec)

	if decayedProb < 0 {
		return 0
	}
	return decayedProb
}
