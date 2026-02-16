package config

import (
	"fmt"
	"testing"

	"github.com/satmihir/fair/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type panicLogger struct{}

func (p *panicLogger) Printf(_ string, _ ...any) {}
func (p *panicLogger) Print(_ ...any)            {}
func (p *panicLogger) Println(_ ...any)          {}
func (p *panicLogger) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}

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

func TestMinFinalProbabilityFunction(t *testing.T) {
	t.Run("returns minimum for non-empty slice", func(t *testing.T) {
		min := MinFinalProbabilityFunction([]float64{0.9, 0.4, 0.7, 0.2})
		require.Equal(t, 0.2, min)
	})

	t.Run("empty slice triggers fatal logger", func(t *testing.T) {
		prevLogger := logger.GetLogger()
		logger.SetLogger(&panicLogger{})
		t.Cleanup(func() {
			logger.SetLogger(prevLogger)
		})

		require.Panics(t, func() {
			MinFinalProbabilityFunction([]float64{})
		})
	})
}

func TestMeanFinalProbabilityFunction(t *testing.T) {
	t.Run("returns mean for non-empty slice", func(t *testing.T) {
		mean := MeanFinalProbabilityFunction([]float64{0.2, 0.4, 0.6, 0.8})
		require.Equal(t, 0.5, mean)
	})

	t.Run("empty slice triggers fatal logger", func(t *testing.T) {
		prevLogger := logger.GetLogger()
		logger.SetLogger(&panicLogger{})
		t.Cleanup(func() {
			logger.SetLogger(prevLogger)
		})

		require.Panics(t, func() {
			MeanFinalProbabilityFunction([]float64{})
		})
	})
}

func TestDefaultFairnessTrackerConfig_GenerateError(t *testing.T) {
	prevGenerator := generateTunedStructureConfig
	prevLogger := logger.GetLogger()

	generateTunedStructureConfig = func(_, _, _ uint32) (*FairnessTrackerConfig, error) {
		return nil, fmt.Errorf("forced generation failure")
	}
	logger.SetLogger(&panicLogger{})

	t.Cleanup(func() {
		generateTunedStructureConfig = prevGenerator
		logger.SetLogger(prevLogger)
	})

	require.Panics(t, func() {
		_ = DefaultFairnessTrackerConfig()
	})
}
