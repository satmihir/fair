package tracker

import (
	"context"
	"testing"
	"time"

	"github.com/satmihir/fair/pkg/request"
	"github.com/stretchr/testify/assert"
)

func TestEndToEnd(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trk, err := trkB.BuildWithDefaultConfig()
	assert.NoError(t, err)
	defer trk.Close()

	ctx := context.Background()
	id := []byte("client_id")

	resp, err := trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.False(t, resp.ShouldThrottle)

	_, err = trk.ReportOutcome(ctx, id, request.OutcomeFailure)
	assert.NoError(t, err)

	resp, err = trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.False(t, resp.ShouldThrottle)

	// 24 failures are enough but there's decay so we will add a few more
	for i := 0; i < 30; i++ {
		_, err = trk.ReportOutcome(ctx, id, request.OutcomeFailure)
		assert.NoError(t, err)
	}

	resp, err = trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.True(t, resp.ShouldThrottle)

	// It takes 10x more failures to get back to 0 probability
	for i := 0; i < 30000; i++ {
		_, err = trk.ReportOutcome(ctx, id, request.OutcomeSuccess)
		assert.NoError(t, err)
	}

	resp, err = trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.False(t, resp.ShouldThrottle)
}

func TestRotation(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trkB.SetRotationFrequency(1 * time.Second)
	trk, err := trkB.Build()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		assert.Equal(t, int(trk.secondaryStructure.GetId()-trk.mainStructure.GetId()), 1)
		time.Sleep(1 * time.Second)
	}

	assert.True(t, trk.secondaryStructure.GetId() >= 2)
}
