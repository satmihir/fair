package tracker

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/data"
	"github.com/satmihir/fair/pkg/utils"
)

func TestOverBudgetConstructors(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.L, cfg.M = 3, 130000

	t.Run("NewStructure", func(t *testing.T) {
		s, err := data.NewStructure(cfg, 1, false)
		require.Error(t, err)
		require.Nil(t, s)
	})
	t.Run("NewStructureWithClock", func(t *testing.T) {
		s, err := data.NewStructureWithClock(cfg, 1, false, utils.NewRealClock())
		require.Error(t, err)
		require.Nil(t, s)
	})
	t.Run("NewFairnessTracker", func(t *testing.T) {
		ft, err := NewFairnessTracker(cfg)
		if ft != nil {
			t.Cleanup(ft.Close)
		}
		require.Error(t, err)
		require.Nil(t, ft)
	})
	t.Run("NewFairnessTrackerWithClockAndTicker", func(t *testing.T) {
		ft, err := NewFairnessTrackerWithClockAndTicker(cfg, utils.NewRealClock(), newFakeTicker())
		if ft != nil {
			t.Cleanup(ft.Close)
		}
		require.Error(t, err)
		require.Nil(t, ft)
	})
	t.Run("GenerateTunedStructureConfig", func(t *testing.T) {
		// One expected bad flow selects the minimum of three levels.
		tuned, err := config.GenerateTunedStructureConfig(1000, cfg.M, 25)
		require.Error(t, err)
		require.Nil(t, tuned)
	})
}

// Run in isolation: go test ./pkg/tracker -run '^$' -bench '^BenchmarkAtLimitRotation$' -benchtime=1x
// TotalAlloc counts cumulative Go heap allocations, including the retired structure even after GC.
func BenchmarkAtLimitRotation(b *testing.B) {
	const budget = 64 << 20
	const levels = 3
	// Largest M at this L satisfying 3*(4096+L*(64+64*M)) <= budget.
	const buckets = ((budget/3-4096)/levels - 64) / 64
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.L, cfg.M = levels, buckets
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ticker := newFakeTicker()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		ft, err := NewFairnessTrackerWithClockAndTicker(cfg, utils.NewRealClock(), ticker)
		require.NoError(b, err)
		ticker.tickAndWait(b, ft)
		runtime.ReadMemStats(&after)
		ft.Close()
		require.Equal(b, uint64(2), ft.mainStructure.GetID())
		require.Equal(b, uint64(3), ft.secondaryStructure.GetID())
		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > budget {
			b.Fatalf("construction and rotation allocated %d bytes; budget is %d", allocated, budget)
		}
	}
}
