package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateL(t *testing.T) {
	// Manually calculated by hand
	L := CalculateL(1000, 100, 0.0001)
	assert.Equal(t, int(L), 4)
}

func TestGenerateTunedStructureConfig(t *testing.T) {
	conf, err := GenerateTunedStructureConfig(1000, 1000, 25)
	assert.NoError(t, err)
	assert.Equal(t, int(conf.L), 3)
	assert.Equal(t, int(conf.M), 1000)
	assert.Equal(t, conf.Pi*25, float64(1))
	assert.Equal(t, conf.Pd*25*1000, float64(1))
}

func TestDefaultStructureConfig(t *testing.T) {
	conf := DefaultFairnessTrackerConfig()
	assert.Equal(t, int(conf.L), 3)
	assert.Equal(t, int(conf.M), 1000)
	assert.Equal(t, conf.Pi*25, float64(1))
	assert.Equal(t, conf.Pd*25*1000, float64(1))
}

func TestGenerateTunedStructureConfigWithZeroTolerance(t *testing.T) {
	// Verify that passing 0 for tolerableBadRequestsPerBadFlow returns an error
	conf, err := GenerateTunedStructureConfig(1000, 1000, 0)

	// Should return an error
	assert.Error(t, err)
	assert.Nil(t, conf)
	assert.Contains(t, err.Error(), "tolerableBadRequestsPerBadFlow must be greater than 0")
}
