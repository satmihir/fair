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

func TestGenerateTunedStructureConfigWithZeroTolerance(t *testing.T) {
	// Verify that passing 0 for tolerableBadRequestsPerBadFlow doesn't panic
	// and falls back to the default value
	conf := GenerateTunedStructureConfig(1000, 1000, 0)

	// Should not panic and should use default value (25)
	assert.NotNil(t, conf)

	// Pi should equal 1/25 = 0.04 (the default)
	expectedPi := 1.0 / 25.0
	assert.Equal(t, expectedPi, conf.Pi)

	// Pd should be pdSlowingFactor * Pi
	expectedPd := 0.001 * expectedPi
	assert.Equal(t, expectedPd, conf.Pd)
}
