package data

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/utils"
	"github.com/spaolacci/murmur3"
)

// Represents a bucket in the leveled structure
type bucket struct {
	// Probability that a request falling on this bucket should be dropped
	probability float64
	// The gace tokens in this bucket that'll save from increasing the probability
	graceTokens uint32
	// The max gace tokens
	graceTokenLimit uint32
	// Time in millis since the bucket was last updated
	lastUpdatedTimeMillis uint64
	// A mutex to protect the state of this bucket from concurrent access
	lock *sync.Mutex
}

func NewBucket(clock utils.IClock, graceTokens uint32) *bucket {
	return &bucket{
		probability:           0,
		lastUpdatedTimeMillis: uint64(clock.Now().UnixMilli()),
		lock:                  &sync.Mutex{},
		graceTokens:           graceTokens,
		graceTokenLimit:       graceTokens,
	}
}

// Implements IStructure with a multi-leveled Bloom filter bucket structure
// to track the throttling probability Pt that starts with 0 for all buckets
// and increases when resource contention is experienced and decreases when
// requests are successful.
type Structure struct {
	// The data at all levels. Every value is a float64 representing the probability
	// of throttling the request.
	levels [][]*bucket
	// The config associated with this structure
	config *config.FairnessTrackerConfig
	// The unique ID of the structure
	id uint64
	// The murmur hash seed
	murmurSeed uint32
	// The clock to use for getting the time
	clock utils.IClock
	// Includes stats in results. Useful for debugging but may slightly affect performance.
	includeStats bool
}

func NewStructureWithClock(config *config.FairnessTrackerConfig, id uint64, includeStats bool, clock utils.IClock) (*Structure, error) {
	if err := validateStructureConfig(config); err != nil {
		return nil, NewDataError(err, "The input config failed validation: %v", config)
	}

	levels := make([][]*bucket, config.L)
	for i := 0; i < int(config.L); i++ {
		levels[i] = make([]*bucket, config.M)

		for j := 0; j < int(config.M); j++ {
			levels[i][j] = NewBucket(clock, config.GraceTokenLimit)
		}
	}

	return &Structure{
		levels:       levels,
		config:       config,
		id:           id,
		murmurSeed:   rand.Uint32(),
		clock:        clock,
		includeStats: includeStats,
	}, nil
}

func NewStructure(config *config.FairnessTrackerConfig, id uint64, includeStats bool) (*Structure, error) {
	return NewStructureWithClock(config, id, includeStats, utils.NewRealClock())
}

func (s *Structure) GetId() uint64 {
	return s.id
}

func (s *Structure) Close() {
}

func (s *Structure) RegisterRequest(ctx context.Context, clientIdentifier []byte) (*request.RegisterRequestResult, error) {
	var stats *request.ResultStats

	bucketProbabilities := make([]float64, s.config.L)

	// We can ignore the error since the handler never returns one
	s.visitBuckets(clientIdentifier, func(l uint32, m uint32, b *bucket) error {
		bucketProbabilities[l] = b.probability
		if s.includeStats {
			if stats == nil {
				stats = &request.ResultStats{
					BucketIndexes: make([]int, s.config.L),
				}
			}
			stats.BucketIndexes[l] = int(m)
		}
		return nil
	})

	pfinal := s.config.FinalProbabilityFunction(bucketProbabilities)

	if s.includeStats {
		stats.BucketProbabilities = bucketProbabilities
		stats.FinalProbability = pfinal
	}

	// Decide whether to throttle the request based on the probability
	shouldThrottle := false
	if rand.Float64() <= pfinal {
		shouldThrottle = true
	}

	return &request.RegisterRequestResult{
		ShouldThrottle: shouldThrottle,
		ResultStats:    stats,
	}, nil
}

func (s *Structure) ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome request.Outcome) (*request.ReportOutcomeResult, error) {
	adjustment := s.config.Pi
	if outcome == request.OutcomeSuccess {
		adjustment = -1 * s.config.Pd
	}

	err := s.visitBuckets(clientIdentifier, func(l uint32, m uint32, b *bucket) error {
		// If the probability is going up, we will try to save it by spending a grace token
		if adjustment > 0 && b.graceTokens > 0 {
			b.graceTokens--
			return nil
		}

		p := b.probability + adjustment
		if p < 0 {
			p = 0
		}

		if p > 1 {
			p = 1
		}

		b.probability = p
		b.lastUpdatedTimeMillis = s.currentMillis()

		// If the probability is going down to 0 (or staying to 0), we will earn a grace token
		if p == 0 {
			b.graceTokens = min(b.graceTokenLimit, b.graceTokens+1)
		}

		return nil
	})

	return &request.ReportOutcomeResult{}, err
}

// Visit the buckets belonging to the given clientIdentifier
// Also takes the bucket lock and manages probability decay prior to calling the handler
func (s *Structure) visitBuckets(clientIdentifier []byte, fn func(uint32, uint32, *bucket) error) error {
	levelHashes := generateNHashesUsing64Bit(clientIdentifier, s.config.L, s.murmurSeed)

	for l := 0; l < int(s.config.L); l++ {
		lvl := s.levels[l]
		m := levelHashes[l] % s.config.M
		buck := lvl[m]

		buck.lock.Lock()

		cur := s.currentMillis()
		deltaT := cur - buck.lastUpdatedTimeMillis
		pm := adjustProbability(buck.probability, s.config.Lambda, deltaT)

		buck.lastUpdatedTimeMillis = cur
		buck.probability = pm

		if err := fn(uint32(l), m, buck); err != nil {
			buck.lock.Unlock()
			return err
		}

		buck.lock.Unlock()
	}

	return nil
}

func (s *Structure) currentMillis() uint64 {
	return uint64(s.clock.Now().UnixMilli())
}

// Validate the input config against invariants
func validateStructureConfig(config *config.FairnessTrackerConfig) error {
	if config.L <= 0 || config.M <= 0 {
		return fmt.Errorf("the values of L and M must be at least 1, found L: %d and M: %d", config.L, config.M)
	}

	if config.Pd <= 0 || config.Pi <= 0 {
		return fmt.Errorf("the values of Pi and Pd must >0, found Pi: %f and Pd: %f", config.Pi, config.Pd)
	}

	if config.Pd > 1 || config.Pi >= 1 {
		return fmt.Errorf("the values of Pi and Pd must <=1, found Pi: %f and Pd: %f", config.Pi, config.Pd)
	}

	// The expectation is we quickly throttle the client when bad things start to happen
	// but cautiously bring it back to avoid retry-storms.
	if config.Pi <= config.Pd {
		return fmt.Errorf("the value of Pd is expected to be smaller than Pi")
	}

	return nil
}

// Calculate n hashes of the given input using murmur hash.
// To optimize, we only calculate a single 64 bit hash and use a technique outlined in
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
	for i := 0; i < int(n); i++ {
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
