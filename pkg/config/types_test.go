package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateStructure(t *testing.T) {
	var c *FairnessTrackerConfig
	require.Error(t, c.ValidateStructure())

	valid := FairnessTrackerConfig{M: 1, L: 1, Pi: 1, Pd: 0.5, Lambda: 0, FinalProbabilityFunction: MinFinalProbabilityFunction}
	require.NoError(t, valid.ValidateStructure()) // No ticker is required by a structure.
	for _, tc := range []struct {
		name   string
		change func(*FairnessTrackerConfig)
	}{
		{"zero M", func(c *FairnessTrackerConfig) { c.M = 0 }},
		{"zero L", func(c *FairnessTrackerConfig) { c.L = 0 }},
		{"zero Pd", func(c *FairnessTrackerConfig) { c.Pd = 0 }},
		{"negative Pd", func(c *FairnessTrackerConfig) { c.Pd = -1 }},
		{"equal increments", func(c *FairnessTrackerConfig) { c.Pi = c.Pd }},
		{"Pi below Pd", func(c *FairnessTrackerConfig) { c.Pi = 0.25 }},
		{"Pi above one", func(c *FairnessTrackerConfig) { c.Pi = math.Nextafter(1, 2) }},
		{"NaN Pd", func(c *FairnessTrackerConfig) { c.Pd = math.NaN() }},
		{"NaN Pi", func(c *FairnessTrackerConfig) { c.Pi = math.NaN() }},
		{"infinite Pi", func(c *FairnessTrackerConfig) { c.Pi = math.Inf(1) }},
		{"infinite Pd", func(c *FairnessTrackerConfig) { c.Pd = math.Inf(-1) }},
		{"negative lambda", func(c *FairnessTrackerConfig) { c.Lambda = -0.01 }},
		{"infinite lambda", func(c *FairnessTrackerConfig) { c.Lambda = math.Inf(1) }},
		{"NaN lambda", func(c *FairnessTrackerConfig) { c.Lambda = math.NaN() }},
		{"nil callback", func(c *FairnessTrackerConfig) { c.FinalProbabilityFunction = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			tc.change(&c)
			require.Error(t, c.ValidateStructure())
		})
	}
}
