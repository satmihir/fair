package tracker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/v2/pkg/config"
	"github.com/satmihir/fair/v2/pkg/request"
	"github.com/satmihir/fair/v2/pkg/utils"
)

// closeRecorder wraps every structure a tracker builds and counts Close calls
// by structure ID.
type closeRecorder struct {
	mu     sync.Mutex
	closes map[uint64]int
	// When set, RegisterRequest on any structure blocks until it is closed.
	hold chan struct{}
	// Signals that a held RegisterRequest has entered the structure.
	entered chan struct{}
}

type recordedStructure struct {
	request.Tracker
	recorder *closeRecorder
}

func (s *recordedStructure) Close() {
	s.recorder.mu.Lock()
	s.recorder.closes[s.GetID()]++
	s.recorder.mu.Unlock()
	s.Tracker.Close()
}

func (s *recordedStructure) RegisterRequest(ctx context.Context, id []byte) *request.RegisterRequestResult {
	if hold := s.recorder.hold; hold != nil {
		s.recorder.entered <- struct{}{}
		<-hold
	}
	return s.Tracker.RegisterRequest(ctx, id)
}

func (r *closeRecorder) count(id uint64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closes[id]
}

// recordStructures makes every structure built during the test report its
// Close calls to the returned recorder.
func recordStructures(t *testing.T) *closeRecorder {
	t.Helper()
	recorder := &closeRecorder{closes: map[uint64]int{}}
	build := newTrackerStructureWithClock
	t.Cleanup(func() { newTrackerStructureWithClock = build })
	newTrackerStructureWithClock = func(cfg *config.FairnessTrackerConfig, id uint64, clock utils.IClock) (request.Tracker, error) {
		s, err := build(cfg, id, clock)
		if err != nil {
			return nil, err
		}
		return &recordedStructure{Tracker: s, recorder: recorder}, nil
	}
	return recorder
}

func TestRotationClosesOnlyTheRetiredStructure(t *testing.T) {
	recorder := recordStructures(t)
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), utils.NewRealClock(), ticker)
	require.NoError(t, err)

	closedOnce := func(id uint64) {
		t.Helper()
		// Close runs just after the rotation is published.
		require.Eventually(t, func() bool { return recorder.count(id) == 1 }, 5*time.Second, time.Millisecond, "structure %d", id)
	}

	ticker.tickAndWait(t, ft) // 1 retires; 2 is active, 3 is the standby.
	closedOnce(1)
	ticker.tickAndWait(t, ft) // 2 retires; 3 is active, 4 is the standby.
	closedOnce(2)
	require.Equal(t, 1, recorder.count(1), "a structure must be closed only once")

	// Closing the tracker leaves the live pair open and usable.
	ft.Close()
	ft.Close()
	require.Zero(t, recorder.count(3))
	require.Zero(t, recorder.count(4))
	ctx, id := context.Background(), []byte("client")
	require.NotNil(t, ft.RegisterRequest(ctx, id))
	require.NotNil(t, ft.ReportOutcome(ctx, id, request.OutcomeSuccess))
}

// A structure must not be closed while a request is still inside it.
func TestRotationWaitsForRequestsBeforeClosing(t *testing.T) {
	recorder := recordStructures(t)
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), utils.NewRealClock(), ticker)
	require.NoError(t, err)
	defer ft.Close()

	recorder.entered = make(chan struct{})
	hold := make(chan struct{})
	recorder.hold = hold

	var inFlight atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		inFlight.Store(true)
		ft.RegisterRequest(context.Background(), []byte("client"))
		inFlight.Store(false)
	}()
	<-recorder.entered // The request is inside structure 1.

	// A rotation comes due while it is there. Do not ask the tracker for its
	// status here: that takes the read lock, which queues behind the waiting
	// rotation, which is waiting for the request this test is holding.
	ticker.ch <- time.Time{}
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, recorder.count(1), "structure closed while a request was using it")
	require.True(t, inFlight.Load())

	close(hold)
	<-done
	require.Eventually(t, func() bool { return recorder.count(1) == 1 }, 5*time.Second, time.Millisecond)
	require.Equal(t, uint64(1), ft.RotationStatus().Rotations)
}

func TestFailedConstructionClosesTheFirstStructure(t *testing.T) {
	recorder := recordStructures(t)
	wrapped := newTrackerStructureWithClock
	newTrackerStructureWithClock = func(cfg *config.FairnessTrackerConfig, id uint64, clock utils.IClock) (request.Tracker, error) {
		if id == 2 {
			return nil, errEntropy
		}
		return wrapped(cfg, id, clock)
	}

	ft, err := NewFairnessTrackerWithClockAndTicker(config.DefaultFairnessTrackerConfig(), utils.NewRealClock(), newFakeTicker())
	require.Nil(t, ft)
	require.ErrorIs(t, err, errEntropy)
	require.Equal(t, 1, recorder.count(1))
}
