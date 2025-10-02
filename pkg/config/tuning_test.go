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
	conf := GenerateTunedStructureConfig(1000, 1000, 25)
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

func TestGenerateTunedStructureConfigZeroTolerable(t *testing.T) {
    conf := GenerateTunedStructureConfig(1000, 1000, 0)
    // Pi should be computed as 1 / defaultTolerableBadRequestsPerBadFlow
    assert.Equal(t, conf.Pi*float64(defaultTolerableBadRequestsPerBadFlow), float64(1))
}