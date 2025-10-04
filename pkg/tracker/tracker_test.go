package tracker

import (
	"context"
	"github.com/satmihir/fair/pkg/testutils"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/satmihir/fair/pkg/request"
)

func TestEndToEnd(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trk, err := trkB.BuildWithDefaultConfig()
	assert.NoError(t, err)
	defer trk.Close()

	ctx := context.Background()
	id := []byte("client_id")

	resp := trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.False(t, resp.ShouldThrottle)

	trk.ReportOutcome(ctx, id, request.OutcomeFailure)

	// 24 failures are enough, but there's decay so we will add a few more
	for i := 0; i < 30; i++ {
		trk.ReportOutcome(ctx, id, request.OutcomeFailure)
	}

	resp = trk.RegisterRequest(ctx, id)
	assert.True(t, resp.ShouldThrottle)

	// It takes 10x more failures to get back to 0 probability
	for i := 0; i < 30000; i++ {
		trk.ReportOutcome(ctx, id, request.OutcomeSuccess)
	}

	resp = trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.False(t, resp.ShouldThrottle)
}

func TestRotation(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trkB.SetRotationFrequency(1 * time.Second)
	trk, err := trkB.Build()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		trk.rotationLock.RLock()
		diff := int(trk.secondaryStructure.GetID() - trk.mainStructure.GetID())
		trk.rotationLock.RUnlock()

		assert.Equal(t, diff, 1)
		time.Sleep(1 * time.Second)
	}

	trk.rotationLock.RLock()
	secID := trk.secondaryStructure.GetID()
	trk.rotationLock.RUnlock()

	assert.True(t, secID >= 2)
}

func TestFairnessTrackerBuilder_BuildWithConfig(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trkDefault, err := trkB.BuildWithDefaultConfig()
	assert.NoError(t, err)
	defer trkDefault.Close()

	trkWithNilConfig, errWithNilConfig := trkB.BuildWithConfig(nil)
	assert.Error(t, errWithNilConfig)
	testutils.TestError(t, &FairnessTrackerError{}, errWithNilConfig, "Configuration cannot be nil", nil)
	assert.Nil(t, trkWithNilConfig)

	trkWithNilConfig, errWithNilConfig = NewFairnessTracker(nil)
	assert.Error(t, errWithNilConfig)
	testutils.TestError(t, &FairnessTrackerError{}, errWithNilConfig, "Configuration cannot be nil", nil)
	assert.Nil(t, trkWithNilConfig)
}
func TestNewFairnessTrackerWithClockAndTicker_NilConfig(t *testing.T) {
	// Passing a nil config should return an error rather than causing a panic.
	ft, err := NewFairnessTrackerWithClockAndTicker(nil, nil, nil)

	assert.Nil(t, ft)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "trackerConfig must not be nil")
	}
}
