package data

import (
	"context"
	"testing"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/stretchr/testify/assert"
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

func TestGetId(t *testing.T) {
	conf := &config.FairnessTrackerConfig{
		L:  2,
		M:  24,
		Pd: .1,
		Pi: .15,
	}
	structure, err := NewStructure(conf, 1, true)
	assert.NoError(t, err)
	assert.NotNil(t, structure)

	assert.Equal(t, int(structure.GetId()), 1)
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

	assert.Equal(t, int(structure.GetId()), 1)

	ctx := context.Background()
	id := []byte("hello_world")

	resp, err := structure.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.ShouldThrottle)

	structure.ReportOutcome(ctx, id, request.OutcomeSuccess)

	resp, err = structure.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.False(t, resp.ShouldThrottle)

	for i := 0; i < 1000; i++ {
		structure.ReportOutcome(ctx, id, request.OutcomeFailure)
	}

	resp, err = structure.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, resp.ShouldThrottle)
}

func TestAdjustProbability(t *testing.T) {
	res := adjustProbability(0.90, .01, 10)
	assert.Equal(t, res, 0.89991000449985)
}
