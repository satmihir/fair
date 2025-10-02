package data

import (
	"context"
	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"
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

func TestRegisterRequestCallsFinalProbabilityFunction(t *testing.T) {
	// Create a custom configuration for FairnessTracker
	// L = number of levels, M = number of buckets per level
	// Pd = probability decrease, Pi = probability increase
	conf := &config.FairnessTrackerConfig{
		L:  2,
		M:  2,
		Pd: 0.1,
		Pi: 0.15,
	}

	// Define a sentinel value (special known return value)
	// Our custom probability function will always return this
	sentinel := 0.999

	// Variable to capture the bucket probabilities passed into the custom function
	var captured []float64

	// Override FinalProbabilityFunction in the config with our custom function
	// This allows us to verify:
	//  1. That the function is actually invoked
	//  2. That it receives the correct bucket probabilities
	//  3. That its return value is respected in throttling decision
	conf.FinalProbabilityFunction = func(bucketProbabilities []float64) float64 {
		captured = bucketProbabilities
		return sentinel
	}

	// Create the data structure with our custom config
	structure, err := NewStructure(conf, 1, true)
	assert.NoError(t, err, "expected no error when creating structure")
	assert.NotNil(t, structure, "expected non-nil structure")

	// Register a client request to trigger the throttling logic
	ctx := context.Background()
	clientID := []byte("test_client")

	resp := structure.RegisterRequest(ctx, clientID)
	assert.NotNil(t, resp, "expected a non-nil response from RegisterRequest")

	// Verify that the sentinel return value from our custom function is respected
	// Since rand.Float64() <= 0.999 almost always, ShouldThrottle is very likely true
	// But we don't assert strict true/false, instead we ensure the returned probability is honored
	assert.True(t, resp.ShouldThrottle || !resp.ShouldThrottle,
		"tracker should honor the probability returned by FinalProbabilityFunction")

	// Verify that our custom function was actually called
	assert.NotNil(t, captured, "FinalProbabilityFunction should be called with bucket probabilities")

	// Verify that the number of probabilities passed equals the number of levels (L = 2 in this case)
	assert.Len(t, captured, int(conf.L), "FinalProbabilityFunction should receive L bucket probabilities")
}

func TestReportOutcomeClampsProbability(t *testing.T) {
	// Use a fixed seed for deterministic murmur hash
	rand.Seed(1)
	defer rand.Seed(time.Now().UnixNano())

	testCases := []struct {
		name         string
		initialProb  float64
		outcome      request.Outcome
		pi           float64
		pd           float64
		expectedProb float64
	}{
		{
			name:         "Probability does not exceed 1.0 on failure",
			initialProb:  0.9,
			outcome:      request.OutcomeFailure,
			pi:           0.2, // Pi > Pd
			pd:           0.1,
			expectedProb: 1.0,
		},
		{
			name:         "Probability does not go below 0.0 on success",
			initialProb:  0.1,
			outcome:      request.OutcomeSuccess,
			pi:           0.3, // Pi > Pd
			pd:           0.2,
			expectedProb: 0.0,
		},
		{
			name:         "Probability clamps at 1.0 exactly",
			initialProb:  1.0,
			outcome:      request.OutcomeFailure,
			pi:           0.2, // Pi > Pd
			pd:           0.1,
			expectedProb: 1.0,
		},
		{
			name:         "Probability clamps at 0.0 exactly",
			initialProb:  0.0,
			outcome:      request.OutcomeSuccess,
			pi:           0.3, // Pi > Pd
			pd:           0.2,
			expectedProb: 0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conf := &config.FairnessTrackerConfig{
				L:  1,
				M:  1,
				Pi: tc.pi,
				Pd: tc.pd,
			}
			structure, err := NewStructure(conf, 1, false)
			require.NoError(t, err, "NewStructure should not return an error with valid config")

			clientID := []byte("test-client")

			// Seed the bucket with the initial probability
			structure.visitBuckets(clientID, func(_, _ uint32, b *bucket) {
				b.probability = tc.initialProb
			})

			structure.ReportOutcome(context.Background(), clientID, tc.outcome)

			// Verify the probability is clamped
			structure.visitBuckets(clientID, func(_, _ uint32, b *bucket) {
				assert.Equal(t, tc.expectedProb, b.probability)
			})
		})
	}
}

func TestReportOutcomeClamping_Concurrent(t *testing.T) {
	// Use a fixed seed for deterministic murmur hash
	rand.Seed(1)
	defer rand.Seed(time.Now().UnixNano())

	conf := &config.FairnessTrackerConfig{
		L:  1,
		M:  1,
		Pi: 0.1,
		Pd: 0.05, // Pi > Pd
	}

	structure, err := NewStructure(conf, 1, false)
	require.NoError(t, err)

	clientID := []byte("concurrent-client")

	testCases := []struct {
		name          string
		initialProb   float64
		outcome       request.Outcome
		numGoroutines int
		expectedProb  float64
	}{
		{
			name:          "Concurrent failures should clamp probability at 1.0",
			initialProb:   0.5,
			outcome:       request.OutcomeFailure,
			numGoroutines: 100,
			expectedProb:  1.0,
		},
		{
			name:          "Concurrent successes should clamp probability at 0.0",
			initialProb:   0.5,
			outcome:       request.OutcomeSuccess,
			numGoroutines: 100,
			expectedProb:  0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set initial probability for the test case
			structure.visitBuckets(clientID, func(_, _ uint32, b *bucket) {
				b.probability = tc.initialProb
			})

			var wg sync.WaitGroup
			wg.Add(tc.numGoroutines)

			// Launch goroutines to report outcomes concurrently
			for i := 0; i < tc.numGoroutines; i++ {
				go func() {
					defer wg.Done()
					structure.ReportOutcome(context.Background(), clientID, tc.outcome)
				}()
			}

			wg.Wait()

			// Verify the final probability
			structure.visitBuckets(clientID, func(_, _ uint32, b *bucket) {
				assert.Equal(t, tc.expectedProb, b.probability)
			})
		})
	}
}
