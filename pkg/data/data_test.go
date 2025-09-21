package data

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
)

func TestValidateStructConfig(t *testing.T) {
	conf := &config.FairnessTrackerConfig{
		L: 0,
	}

	err := validateStructureConfig(conf)
	assert.Error(t, err)

	conf = &config.FairnessTrackerConfig{
		L: 1,
		M: 0,
	}

	err = validateStructureConfig(conf)
	assert.Error(t, err)

	conf = &config.FairnessTrackerConfig{
		L:  1,
		M:  1,
		Pd: 0,
		Pi: 0,
	}

	err = validateStructureConfig(conf)
	assert.Error(t, err)

	conf = &config.FairnessTrackerConfig{
		L:  1,
		M:  1,
		Pd: 10,
		Pi: 10,
	}

	err = validateStructureConfig(conf)
	assert.Error(t, err)

	conf = &config.FairnessTrackerConfig{
		L:  1,
		M:  1,
		Pd: .15,
		Pi: .1,
	}

	err = validateStructureConfig(conf)
	assert.Error(t, err)

	conf = &config.FairnessTrackerConfig{
		L:  1,
		M:  1,
		Pd: .1,
		Pi: .15,
	}

	err = validateStructureConfig(conf)
	assert.NoError(t, err)
}

func TestNewStructureFailsValidation(t *testing.T) {
	conf := &config.FairnessTrackerConfig{
		L:  1,
		M:  1,
		Pd: .15,
		Pi: .1,
	}
	_, err := NewStructure(conf, 1, true)
	assert.Error(t, err)
}

func TestNewStructure(t *testing.T) {
	conf := &config.FairnessTrackerConfig{
		L:  2,
		M:  24,
		Pd: .1,
		Pi: .15,
	}
	structure, err := NewStructure(conf, 1, true)
	assert.NoError(t, err)
	assert.NotNil(t, structure)

	assert.Equal(t, len(structure.levels), 2)
	assert.Equal(t, len(structure.levels[0]), 24)
}

func TestHashes(t *testing.T) {
	datum := []byte("hello world")
	hashes := generateNHashesUsing64Bit(datum, 3, 5)

	assert.Equal(t, len(hashes), 3)

	hashes2 := generateNHashesUsing64Bit(datum, 3, 5)

	assert.Equal(t, hashes[0], hashes2[0])
	assert.Equal(t, hashes[1], hashes2[1])
	assert.Equal(t, hashes[2], hashes2[2])
}

func TestGetID(t *testing.T) {
	conf := &config.FairnessTrackerConfig{
		L:  2,
		M:  24,
		Pd: .1,
		Pi: .15,
	}
	structure, err := NewStructure(conf, 1, true)
	assert.NoError(t, err)
	assert.NotNil(t, structure)

	assert.Equal(t, int(structure.GetID()), 1)
}

func TestEndToEnd(t *testing.T) {
	conf := &config.FairnessTrackerConfig{
		L:                        2,
		M:                        24,
		Pd:                       .1,
		Pi:                       .15,
		Lambda:                   0,
		FinalProbabilityFunction: config.MeanFinalProbabilityFunction,
	}
	structure, err := NewStructure(conf, 1, true)
	assert.NoError(t, err)
	assert.NotNil(t, structure)

	assert.Equal(t, int(structure.GetID()), 1)

	ctx := context.Background()
	id := []byte("hello_world")

	resp := structure.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.ShouldThrottle)

	structure.ReportOutcome(ctx, id, request.OutcomeSuccess)

	resp = structure.RegisterRequest(ctx, id)
	assert.NotNil(t, resp)
	assert.False(t, resp.ShouldThrottle)

	for i := 0; i < 1000; i++ {
		structure.ReportOutcome(ctx, id, request.OutcomeFailure)
	}

	resp = structure.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.ShouldThrottle)
}

// TestAdjustProbability verifies that the adjustProbability function
// correctly applies exponential decay across a range of scenarios.
//
// Covered cases:
//  1. No decay occurs when lambda is zero (decay rate = 0).
//  2. No decay occurs when deltaMs is zero (no time elapsed).
//  3. Large deltaMs values push probability toward zero.
//  4. A starting probability of zero always stays zero.
//  5. Small positive decay applies correctly for short time intervals.
func TestAdjustProbability(t *testing.T) {
	tests := []struct {
        name     string
        prob     float64
        lambda   float64
        deltaMs  uint64
        expected float64
    }{
        {
            name:     "No decay when lambda is 0",
            prob:     0.8,
            lambda:   0,
            deltaMs:  10000,
            expected: 0.8,
        },
        {
            name:     "No decay when deltaMs is 0",
            prob:     0.6,
            lambda:   0.5,
            deltaMs:  0,
            expected: 0.6,
        },
        {
            name:     "Decay approaches 0 for large deltaMs",
            prob:     0.9,
            lambda:   1.0,
            deltaMs:  1000000, // very large time
            expected: 0.0,     // should be nearly 0
        },
        {
            name:     "Probability stays 0 if starting from 0",
            prob:     0,
            lambda:   1.0,
            deltaMs:  5000,
            expected: 0,
        },
        {
            name:     "Small decay with short delta",
            prob:     1.0,
            lambda:   0.1,
            deltaMs:  100,
            expected: 1.0 * math.Exp(-0.1*0.1), // e^(-0.01)
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := adjustProbability(tt.prob, tt.lambda, tt.deltaMs)

            // For float comparisons use tolerance
            if math.Abs(got-tt.expected) > 1e-6 {
                t.Errorf("adjustProbability() = %v, want %v", got, tt.expected)
            }
        })
    }
}

// Explicitly test nil config case
func TestValidateStructConfig_NilConfig(t *testing.T) {
	err := validateStructureConfig(nil)

	// Expect an error instead of panic
	assert.Error(t, err)

	// Ensure we wrapped it in DataError (consistent with other failures)
	assert.IsType(t, &DataError{}, err)

	// Error message should clearly state the root cause
	assert.Contains(t, err.Error(), "cannot be nil")
}
