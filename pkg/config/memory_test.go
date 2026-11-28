package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRotationBudgetBoundaries(t *testing.T) {
	// Maximum width for three levels at 64 MiB, including row and fixed charges.
	const maxWidth = (MaxRotationBytes/3 - 4096 - 3*64) / (3 * 64)
	cfg := DefaultFairnessTrackerConfig()
	cfg.L, cfg.M = 3, maxWidth
	require.NoError(t, cfg.ValidateStructure())
	cfg.M++
	require.ErrorContains(t, cfg.ValidateStructure(), "rotation storage budget")
	cfg.L, cfg.M = math.MaxUint32, math.MaxUint32
	require.ErrorContains(t, cfg.ValidateStructure(), "rotation storage budget")
}
