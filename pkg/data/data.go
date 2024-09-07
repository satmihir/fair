package data

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/spaolacci/murmur3"
)

// Represents a bucket in the leveled structure
type bucket struct {
	// Probability that a request falling on this bucket should be dropped
	probability float64
	// Time in millis since the bucket was last updated
	lastUpdatedTimeMillis uint64
	// A mutex to protect the state of this bucket from concurrent access
	lock *sync.Mutex
}

func NewBucket() *bucket {
	return &bucket{
		probability:           0,
		lastUpdatedTimeMillis: currentMillis(),
		lock:                  &sync.Mutex{},
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
	config *StructureConfig
	// The unique ID of the structure
	id uint32
	// The murmur hash seed
	murmurSeed uint32
}

func NewStructure(config *StructureConfig, id uint32) (*Structure, error) {
	if err := validateStructureConfig(config); err != nil {
		return nil, NewDataError(err, "The input config failed validation: %v", config)
	}

	levels := make([][]*bucket, config.L)
	for i := 0; i < int(config.L); i++ {
		levels[i] = make([]*bucket, config.M)

		for j := 0; j < int(config.M); j++ {
			levels[i][j] = NewBucket()
		}
	}

	return &Structure{
		levels:     levels,
		config:     config,
		id:         id,
		murmurSeed: rand.Uint32(),
	}, nil
}

func (s *Structure) GetId() uint32 {
	return s.id
}

func (s *Structure) RegisterRequest(ctx context.Context, clientIdentifier []byte) (*RegisterResponse, error) {
	var pmin float64 = 1

	// We can ignore the error since the handler never returns one
	s.visitBuckets(clientIdentifier, func(b *bucket) error {
		if b.probability < pmin {
			pmin = b.probability
		}

		return nil
	})

	// Decide whether to throttle the request based on the probability
	shouldThrottle := false
	if rand.Float64() <= pmin {
		shouldThrottle = true
	}

	return &RegisterResponse{
		ShouldThrottle: shouldThrottle,
	}, nil
}

func (s *Structure) ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome Outcome) error {
	adjustment := s.config.Pi
	if outcome == OutcomeSuccess {
		adjustment = -1 * s.config.Pd
	}

	return s.visitBuckets(clientIdentifier, func(b *bucket) error {
		p := b.probability + adjustment
		if p < 0 {
			p = 0
		}

		if p > 1 {
			p = 1
		}

		b.probability = p
		b.lastUpdatedTimeMillis = currentMillis()

		return nil
	})
}

// Visit the buckets belonging to the given clientIdentifier
// Also takes the bucket lock and manages probability decay prior to calling the handler
func (s *Structure) visitBuckets(clientIdentifier []byte, fn func(*bucket) error) error {
	levelHashes := generateNHashesUsing64Bit(clientIdentifier, s.config.L, s.murmurSeed)

	for l := 0; l < int(s.config.L); l++ {
		lvl := s.levels[l]
		buck := lvl[levelHashes[l]%s.config.M]

		buck.lock.Lock()

		cur := currentMillis()
		deltaT := cur - buck.lastUpdatedTimeMillis
		pm := adjustProbability(buck.probability, s.config.lambda, deltaT)

		buck.lastUpdatedTimeMillis = cur
		buck.probability = pm

		if err := fn(buck); err != nil {
			buck.lock.Unlock()
			return err
		}

		buck.lock.Unlock()
	}

	return nil
}

// Validate the input config against invariants
func validateStructureConfig(config *StructureConfig) error {
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

func currentMillis() uint64 {
	return uint64(time.Now().UnixMilli())
}
