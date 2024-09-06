package data

import (
	"fmt"

	"github.com/spaolacci/murmur3"
)

// Implements IStructure with a multi-leveled Bloom filter bucket structure
// to track the throttling probability Pt that starts with 0 for all buckets
// and increases when resource contention is experienced and decreases when
// requests are successful.
type Structure struct {
	// The data at all levels. Every value is a float64 representing the probability
	// of throttling the request.
	levels [][]float64
}

func NewStructure(config *StructureConfig) (*Structure, error) {
	if err := validateStructureConfig(config); err != nil {
		return nil, NewDataError(err, "The input config failed validation: %v", config)
	}

	levels := make([][]float64, config.L)
	for i := 0; i < int(config.L); i++ {
		levels[i] = make([]float64, config.M)
	}

	return &Structure{
		levels: levels,
	}, nil
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
func GenerateNHashesUsing64Bit(input []byte, n int) []uint32 {
	// Compute the 64-bit hash
	hash64 := murmur3.Sum64(input)

	// Split the 64-bit hash into two 32-bit hashes
	hash1 := uint32(hash64)       // Lower 32 bits
	hash2 := uint32(hash64 >> 32) // Upper 32 bits

	// Generate the n hashes using the combination: hash_i = hash1 + i * hash2
	hashes := make([]uint32, n)
	for i := 0; i < n; i++ {
		hashes[i] = hash1 + uint32(i)*hash2
	}

	return hashes
}
