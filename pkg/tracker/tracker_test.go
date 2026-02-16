package tracker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/logger"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/testutils"
	"github.com/satmihir/fair/pkg/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRotation(t *testing.T) {
	trkB := NewFairnessTrackerBuilder()
	trkB.SetRotationFrequency(1 * time.Second)
	trk, err := trkB.Build()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		trk.rotationLock.RLock()
		diff := int(trk.secondaryStructure.GetID() - trk.mainStructure.GetID())
		trk.rotationLock.RUnlock()

		assert.Equal(t, diff, 1)
		time.Sleep(1 * time.Second)
	}

	trk.rotationLock.RLock()
	secID := trk.secondaryStructure.GetID()
	trk.rotationLock.RUnlock()

	assert.True(t, secID >= 2)
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
	ft, err := NewFairnessTrackerWithClockAndTicker(nil, nil, nil)

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

type fatalCaptureLogger struct {
	fatalCh chan string
}

func (l *fatalCaptureLogger) Printf(_ string, _ ...any) {}
func (l *fatalCaptureLogger) Print(_ ...any)            {}
func (l *fatalCaptureLogger) Println(_ ...any)          {}
func (l *fatalCaptureLogger) Fatalf(format string, args ...any) {
	select {
	case l.fatalCh <- fmt.Sprintf(format, args...):
	default:
	}
}

func TestNewFairnessTrackerWithClockAndTicker_FirstStructureError(t *testing.T) {
	prevConstructor := newTrackerStructureWithClock
	t.Cleanup(func() {
		newTrackerStructureWithClock = prevConstructor
	})

	newTrackerStructureWithClock = func(_ *config.FairnessTrackerConfig, _ uint64, _ bool, _ utils.IClock) (request.Tracker, error) {
		return nil, fmt.Errorf("first structure failed")
	}

	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), nil, nil)
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

	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), nil, nil)
	require.Nil(t, ft)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Failed to create a structure")
}

func TestNewFairnessTrackerWithClockAndTicker_RotationStructureError(t *testing.T) {
	prevConstructor := newTrackerStructureWithClock
	prevLogger := logger.GetLogger()
	t.Cleanup(func() {
		newTrackerStructureWithClock = prevConstructor
		logger.SetLogger(prevLogger)
	})

	fatalCh := make(chan string, 1)
	logger.SetLogger(&fatalCaptureLogger{fatalCh: fatalCh})

	call := 0
	newTrackerStructureWithClock = func(_ *config.FairnessTrackerConfig, id uint64, _ bool, _ utils.IClock) (request.Tracker, error) {
		call++
		if call <= 2 {
			return &fakeTracker{id: id}, nil
		}
		return nil, fmt.Errorf("rotation creation failed")
	}

	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), nil, ticker)
	require.NoError(t, err)
	require.NotNil(t, ft)

	ticker.ch <- time.Now()

	select {
	case msg := <-fatalCh:
		require.Contains(t, msg, "failed to create a structure during rotation")
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected fatal log during rotation failure")
	}

	ft.Close()
	require.True(t, ticker.stopped)
}
