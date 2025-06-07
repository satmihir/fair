package tracker

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/testutils"
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
	assert.NoError(t, err)
	assert.Equal(t, int(tr.trackerConfig.L), 10)
	assert.Equal(t, int(tr.trackerConfig.M), 10)
}

func TestBuildWithConfig(t *testing.T) {
	c := config.GenerateTunedStructureConfig(10, 10, 10)
	b := NewFairnessTrackerBuilder()
	tr, err := b.BuildWithConfig(c)
	assert.NoError(t, err)
	assert.Equal(t, int(tr.trackerConfig.L), 4)
	assert.Equal(t, int(tr.trackerConfig.M), 10)
}

func TestBuildWithNilConfig(t *testing.T) {
	b := NewFairnessTrackerBuilder()
	tr, err := b.BuildWithConfig(nil)
	assert.Nil(t, tr)
	assert.Error(t, err)
}
