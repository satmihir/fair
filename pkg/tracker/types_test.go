package tracker

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/v2/pkg/config"
	"github.com/satmihir/fair/v2/pkg/request"
	"github.com/satmihir/fair/v2/pkg/testutils"
)

func TestFairnessTrackerError(t *testing.T) {
	origErr := fmt.Errorf("original error")
	dataErr := NewFairnessTrackerError(origErr, "data error occurred")

	testutils.TestError(t, &FairnessTrackerError{}, dataErr, "data error occurred: original error", origErr)
}

func TestBuildFairnessTracker(t *testing.T) {
	b := NewFairnessTrackerBuilder()
	b.SetL(10)
	b.SetM(10)
	b.SetPd(.1)
	b.SetPi(.2)
	b.SetLambda(.001)
	b.SetRotationFrequency(1 * time.Second)
	b.SetIncludeStats(true)
	b.SetFinalProbabilityFunction(config.MeanFinalProbabilityFunction)

	tr, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(tr.Close)
	assert.Equal(t, int(tr.trackerConfig.L), 10)
	assert.Equal(t, int(tr.trackerConfig.M), 10)
	assert.Equal(t, 1*time.Second, tr.trackerConfig.RotationFrequency,
		"rotation frequency should match the value set via builder")
}

func TestNewFairnessTrackerWithTunedConfig(t *testing.T) {
	c, err := config.GenerateTunedStructureConfig(10, 10, 10)
	assert.NoError(t, err)
	tr, err := NewFairnessTracker(c)
	require.NoError(t, err)
	t.Cleanup(tr.Close)
	assert.Equal(t, int(tr.trackerConfig.L), 4)
	assert.Equal(t, int(tr.trackerConfig.M), 10)
}

func TestBuilderReusePreservesTrackerConfig(t *testing.T) {
	b := NewFairnessTrackerBuilder()
	b.SetIncludeStats(true)
	b.SetFinalProbabilityFunction(func([]float64) float64 { return 0.25 })
	first, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(first.Close)

	b.SetFinalProbabilityFunction(func([]float64) float64 { return 0.75 })
	second, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(second.Close)

	ctx, id := context.Background(), []byte("client")
	firstStats := first.RegisterRequest(ctx, id).ResultStats
	secondStats := second.RegisterRequest(ctx, id).ResultStats
	require.NotNil(t, firstStats)
	require.NotNil(t, secondStats)
	require.Equal(t, 0.25, firstStats.FinalProbability)
	require.Equal(t, 0.75, secondStats.FinalProbability)
}

func TestBuilderSetHashKeySource(t *testing.T) {
	placement := func() []int {
		b := NewFairnessTrackerBuilder()
		b.SetIncludeStats(true)
		b.SetHashKeySource(rand.New(rand.NewSource(11)))
		tr, err := b.Build()
		require.NoError(t, err)
		defer tr.Close()
		return tr.RegisterRequest(context.Background(), []byte("client")).ResultStats.BucketIndexes
	}
	require.Equal(t, placement(), placement(), "a seeded key source must fix placement")
}

func TestBuilderSetDecisionSource(t *testing.T) {
	draw := 1.
	b := NewFairnessTrackerBuilder()
	b.SetPi(.5)
	b.SetPd(.25)
	b.SetDecisionSource(func() float64 { return draw })
	tr, err := b.Build()
	require.NoError(t, err)
	defer tr.Close()

	ctx, id := context.Background(), []byte("client")
	tr.ReportOutcome(ctx, id, request.OutcomeFailure) // Probability 0.5.
	require.False(t, tr.RegisterRequest(ctx, id).ShouldThrottle)
	draw = 0
	require.True(t, tr.RegisterRequest(ctx, id).ShouldThrottle)
}
