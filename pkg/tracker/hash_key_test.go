package tracker

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
)

var errEntropy = errors.New("entropy unavailable")

// flakyKeys serves keys from a fixed sequence until told to fail.
type flakyKeys struct {
	keys    *rand.Rand
	failing atomic.Bool
}

func (f *flakyKeys) Read(p []byte) (int, error) {
	if f.failing.Load() {
		return 0, errEntropy
	}
	return f.keys.Read(p)
}

func TestKeyGenerationFailureFailsTrackerConstruction(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	keys := &flakyKeys{keys: rand.New(rand.NewSource(1))}
	keys.failing.Store(true)
	cfg.HashKeySource = keys

	ft, err := NewFairnessTracker(cfg)
	require.Nil(t, ft)
	var trackerErr *FairnessTrackerError
	require.ErrorAs(t, err, &trackerErr)
	require.ErrorIs(t, err, errEntropy)

	ft, err = NewFairnessTrackerWithClockAndTicker(cfg, &manualClock{now: time.Unix(1_700_000_000, 0)}, newFakeTicker())
	require.Nil(t, ft)
	require.ErrorIs(t, err, errEntropy)
}

// A rotation that cannot get a key keeps the current pair serving, reports
// the failure, and succeeds once keys are available again.
func TestRotationSurvivesKeyGenerationFailure(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.Pi, cfg.Pd, cfg.Lambda = .5, .25, 0
	cfg.IncludeStats = true
	keys := &flakyKeys{keys: rand.New(rand.NewSource(1))}
	cfg.HashKeySource = keys
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, &manualClock{now: time.Unix(1_700_000_000, 0)}, ticker)
	require.NoError(t, err)
	defer ft.Close()

	ctx, id := context.Background(), []byte("client")
	probability := func() float64 { return ft.RegisterRequest(ctx, id).ResultStats.FinalProbability }
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	require.Equal(t, .5, probability())

	keys.failing.Store(true)
	ticker.tickAndWait(t, ft)
	status := ft.RotationStatus()
	require.Equal(t, uint64(0), status.Rotations)
	require.Equal(t, uint64(1), status.Failures)
	require.ErrorIs(t, status.LastError, errEntropy)
	require.Equal(t, .5, probability(), "the current pair must keep serving")

	keys.failing.Store(false)
	ticker.tickAndWait(t, ft)
	status = ft.RotationStatus()
	require.Equal(t, uint64(1), status.Rotations)
	require.Equal(t, uint64(1), status.Failures)
	require.NoError(t, status.LastError)
	require.Equal(t, .5, probability(), "the warm standby took over")
}

// The config is copied at construction, but the key source is shared: every
// generation of one tracker reads its key from the same reader, in order.
func TestInjectedKeysMakePlacementReproducible(t *testing.T) {
	rows := func() [][]int {
		cfg := config.DefaultFairnessTrackerConfig()
		cfg.IncludeStats = true
		cfg.HashKeySource = rand.New(rand.NewSource(7))
		ticker := newFakeTicker()
		ft, err := NewFairnessTrackerWithClockAndTicker(cfg, &manualClock{now: time.Unix(1_700_000_000, 0)}, ticker)
		require.NoError(t, err)
		defer ft.Close()

		var seen [][]int
		for generation := 0; generation < 4; generation++ {
			seen = append(seen, ft.RegisterRequest(context.Background(), []byte("client")).ResultStats.BucketIndexes)
			ticker.tickAndWait(t, ft)
		}
		return seen
	}

	first := rows()
	require.Equal(t, first, rows(), "the same key sequence must give the same placement")
	for generation := 1; generation < len(first); generation++ {
		require.NotEqual(t, first[generation-1], first[generation], "rotation must change placement")
	}
}
