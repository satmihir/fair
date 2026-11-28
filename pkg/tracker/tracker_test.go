package tracker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/testutils"
	"github.com/satmihir/fair/pkg/utils"
)

func TestEndToEnd(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trk, err := trkB.BuildWithDefaultConfig()
	assert.NoError(t, err)
	defer trk.Close()

	ctx := context.Background()
	id := []byte("client_id")

	resp := trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.False(t, resp.ShouldThrottle)

	trk.ReportOutcome(ctx, id, request.OutcomeFailure)

	// 24 failures are enough, but there's decay so we will add a few more
	for i := 0; i < 30; i++ {
		trk.ReportOutcome(ctx, id, request.OutcomeFailure)
	}

	resp = trk.RegisterRequest(ctx, id)
	assert.True(t, resp.ShouldThrottle)

	// It takes 10x more failures to get back to 0 probability
	for i := 0; i < 30000; i++ {
		trk.ReportOutcome(ctx, id, request.OutcomeSuccess)
	}

	resp = trk.RegisterRequest(ctx, id)
	assert.NoError(t, err)
	assert.False(t, resp.ShouldThrottle)
}

func TestRotationUsesConfigSnapshot(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.RotationFrequency = 0 // The injected ticker controls rotation.
	cfg.IncludeStats = true
	cfg.FinalProbabilityFunction = func([]float64) float64 { return 0.25 }
	ticker := newFakeTicker()
	trk, err := NewFairnessTrackerWithClockAndTicker(cfg, utils.NewRealClock(), ticker)
	require.NoError(t, err)
	t.Cleanup(trk.Close)

	cfg.IncludeStats = false
	cfg.FinalProbabilityFunction = func([]float64) float64 { return 0.75 }
	// After two ticks, the structure created on the first tick is serving requests.
	for mainID := uint64(2); mainID <= 3; mainID++ {
		ticker.ch <- time.Now()
		require.Eventually(t, func() bool {
			trk.rotationLock.RLock()
			defer trk.rotationLock.RUnlock()
			return trk.mainStructure.GetID() == mainID && trk.secondaryStructure.GetID() == mainID+1
		}, time.Second, time.Millisecond)
		resp := trk.RegisterRequest(context.Background(), []byte("client"))
		require.NotNil(t, resp.ResultStats)
		require.Equal(t, 0.25, resp.ResultStats.FinalProbability)
	}
}

func TestFairnessTrackerBuilder_BuildWithConfig(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trkDefault, err := trkB.BuildWithDefaultConfig()
	assert.NoError(t, err)
	defer trkDefault.Close()

	trkWithNilConfig, errWithNilConfig := trkB.BuildWithConfig(nil)
	assert.Error(t, errWithNilConfig)
	testutils.TestError(t, &FairnessTrackerError{}, errWithNilConfig, "Configuration cannot be nil", nil)
	assert.Nil(t, trkWithNilConfig)

	trkWithNilConfig, errWithNilConfig = NewFairnessTracker(nil)
	assert.Error(t, errWithNilConfig)
	testutils.TestError(t, &FairnessTrackerError{}, errWithNilConfig, "Configuration cannot be nil", nil)
	assert.Nil(t, trkWithNilConfig)
}
func TestNewFairnessTrackerWithClockAndTicker_NilConfig(t *testing.T) {
	// Passing a nil config should return an error rather than causing a panic.
	ft, err := NewFairnessTrackerWithClockAndTicker(nil, utils.NewRealClock(), newFakeTicker())

	assert.Nil(t, ft)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "trackerConfig must not be nil")
	}
}

type fakeTicker struct {
	ch      chan time.Time
	stopped bool
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{
		ch: make(chan time.Time, 1),
	}
}

func (f *fakeTicker) C() <-chan time.Time {
	return f.ch
}

func (f *fakeTicker) Stop() {
	f.stopped = true
}

type fakeTracker struct {
	id uint64
}

func (f *fakeTracker) GetID() uint64 {
	return f.id
}

func (f *fakeTracker) RegisterRequest(_ context.Context, _ []byte) *request.RegisterRequestResult {
	return &request.RegisterRequestResult{}
}

func (f *fakeTracker) ReportOutcome(_ context.Context, _ []byte, _ request.Outcome) *request.ReportOutcomeResult {
	return &request.ReportOutcomeResult{}
}

func (f *fakeTracker) Close() {}

func TestNewFairnessTrackerWithClockAndTicker_FirstStructureError(t *testing.T) {
	prevConstructor := newTrackerStructureWithClock
	t.Cleanup(func() {
		newTrackerStructureWithClock = prevConstructor
	})

	newTrackerStructureWithClock = func(_ *config.FairnessTrackerConfig, _ uint64, _ bool, _ utils.IClock) (request.Tracker, error) {
		return nil, fmt.Errorf("first structure failed")
	}

	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), utils.NewRealClock(), ticker)
	require.False(t, ticker.stopped, "caller retains the ticker on failure")
	require.Nil(t, ft)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to create a structure")
}

func TestNewFairnessTrackerWithClockAndTicker_SecondStructureError(t *testing.T) {
	prevConstructor := newTrackerStructureWithClock
	t.Cleanup(func() {
		newTrackerStructureWithClock = prevConstructor
	})

	call := 0
	newTrackerStructureWithClock = func(_ *config.FairnessTrackerConfig, id uint64, _ bool, _ utils.IClock) (request.Tracker, error) {
		call++
		if call == 1 {
			return &fakeTracker{id: id}, nil
		}
		return nil, fmt.Errorf("second structure failed")
	}

	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), utils.NewRealClock(), ticker)
	require.False(t, ticker.stopped, "caller retains the ticker on failure")
	require.Nil(t, ft)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to create a structure")
}

func TestRotationRecoversAfterStructureError(t *testing.T) {
	newStructure := newTrackerStructureWithClock
	t.Cleanup(func() { newTrackerStructureWithClock = newStructure })
	errRotation := fmt.Errorf("rotation creation failed")
	calls := 0
	newTrackerStructureWithClock = func(cfg *config.FairnessTrackerConfig, id uint64, stats bool, clock utils.IClock) (request.Tracker, error) {
		calls++
		if calls == 3 { // First rotation, after both initial structures.
			return nil, errRotation
		}
		return newStructure(cfg, id, stats, clock)
	}

	cfg := config.DefaultFairnessTrackerConfig()
	cfg.IncludeStats = true
	cfg.Lambda = 0 // Keep probabilities exact across manual ticks.
	cfg.Pi = 0.25
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, utils.NewRealClock(), ticker)
	require.NoError(t, err)
	t.Cleanup(ft.Close)

	checkIDs := func(mainID, secondaryID uint64) {
		t.Helper()
		ft.rotationLock.RLock()
		defer ft.rotationLock.RUnlock()
		require.Equal(t, mainID, ft.mainStructure.GetID())
		require.Equal(t, secondaryID, ft.secondaryStructure.GetID())
	}
	ctx, client := context.Background(), []byte("client")
	checkProbability := func(want float64) {
		t.Helper()
		stats := ft.RegisterRequest(ctx, client).ResultStats
		require.NotNil(t, stats)
		require.Equal(t, want, stats.FinalProbability)
	}
	checkIDs(1, 2)
	status := ft.RotationStatus()
	require.Zero(t, status.Failures)
	require.NoError(t, status.LastError)
	ft.ReportOutcome(ctx, client, request.OutcomeFailure)
	checkProbability(0.25)

	ticker.ch <- time.Now()
	require.Eventually(t, func() bool { return ft.RotationStatus().Failures != 0 }, time.Second, time.Millisecond)
	status = ft.RotationStatus()
	require.Equal(t, uint64(1), status.Failures)
	require.ErrorContains(t, status.LastError, errRotation.Error())
	checkIDs(1, 2)
	checkProbability(0.25)
	ft.ReportOutcome(ctx, client, request.OutcomeFailure)
	checkProbability(0.5)

	ticker.ch <- time.Now()
	require.Eventually(t, func() bool { return ft.RotationStatus().LastError == nil }, time.Second, time.Millisecond)
	status = ft.RotationStatus()
	require.Equal(t, uint64(1), status.Failures)
	require.NoError(t, status.LastError)
	checkIDs(2, 3) // Promote the warm standby and reuse the failed candidate's ID.
	checkProbability(0.5)
}

func TestNewFairnessTrackerRejectsInvalidRotationFrequency(t *testing.T) {
	for _, frequency := range []time.Duration{0, -time.Second} {
		t.Run(frequency.String(), func(t *testing.T) {
			cfg := config.DefaultFairnessTrackerConfig()
			cfg.RotationFrequency = frequency
			trk, err := NewFairnessTracker(cfg)
			require.ErrorAs(t, err, new(*FairnessTrackerError))
			require.Nil(t, trk)
		})
	}
}

func TestNewFairnessTrackerRejectsNilCallback(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.FinalProbabilityFunction = nil
	trk, err := NewFairnessTracker(cfg)
	require.ErrorAs(t, err, new(*FairnessTrackerError))
	require.Nil(t, trk)
}

func TestNewFairnessTrackerRejectsNilDependencies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		clock  utils.IClock
		ticker utils.ITicker
	}{
		{"nil clock", nil, newFakeTicker()},
		{"typed nil clock", (*utils.Clock)(nil), newFakeTicker()},
		{"nil ticker", utils.NewRealClock(), nil},
		{"typed nil ticker", utils.NewRealClock(), (*fakeTicker)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trk, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), tc.clock, tc.ticker)
			require.ErrorAs(t, err, new(*FairnessTrackerError))
			require.Nil(t, trk)
		})
	}
}
