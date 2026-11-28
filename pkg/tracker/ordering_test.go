package tracker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/utils"
)

// Pause the failure after main has applied it, but before standby does.
type pausedStandby struct {
	request.Tracker
	entered chan struct{}
	resume  chan struct{}
}

func (s *pausedStandby) ReportOutcome(ctx context.Context, id []byte, outcome request.Outcome) *request.ReportOutcomeResult {
	if outcome == request.OutcomeFailure {
		close(s.entered)
		<-s.resume
	}
	return s.Tracker.ReportOutcome(ctx, id, outcome)
}

func TestOverlappingOutcomesSurvivePromotion(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.M, cfg.L = 1, 1
	cfg.Pi, cfg.Pd, cfg.Lambda = 0.5, 0.25, 0
	cfg.IncludeStats = true
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, utils.NewRealClock(), ticker)
	require.NoError(t, err)
	defer ft.Close()

	ft.rotationLock.Lock()
	standby := &pausedStandby{ft.secondaryStructure, make(chan struct{}), make(chan struct{})}
	ft.secondaryStructure = standby
	ft.rotationLock.Unlock()

	ctx := context.Background()
	id := []byte("client")
	failureDone := make(chan struct{})
	go func() {
		ft.ReportOutcome(ctx, id, request.OutcomeFailure)
		close(failureDone)
	}()
	<-standby.entered

	// If the first report doesn't own the client stripe at this point, let
	// success overtake it on standby. Otherwise waiting for success here
	// would deadlock. Probe before starting success so only failure could
	// hold the stripe.
	stripe := ft.outcomeLock(id)
	serialized := !stripe.TryLock()
	if !serialized {
		stripe.Unlock()
	}
	started, successDone := make(chan struct{}), make(chan struct{})
	go func() {
		close(started)
		ft.ReportOutcome(ctx, []byte("client"), request.OutcomeSuccess)
		close(successDone)
	}()
	<-started
	if !serialized {
		<-successDone
	}
	close(standby.resume)
	<-failureDone
	<-successDone

	// Failure then success gives .25; success then failure gives .5
	// because a success at zero is clamped. Promotion must preserve .25.
	probability := func() float64 { return ft.RegisterRequest(ctx, id).ResultStats.FinalProbability }
	require.Equal(t, 0.25, probability())
	ticker.tickAndWait(t, ft)
	require.Same(t, standby, ft.mainStructure)
	require.Equal(t, 0.25, probability(), "promoted standby applied outcomes out of order")
}
