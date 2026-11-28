package tracker

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
)

type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) Sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type countingStandby struct {
	request.Tracker
	registers atomic.Int32
}

func (s *countingStandby) RegisterRequest(ctx context.Context, id []byte) *request.RegisterRequestResult {
	s.registers.Add(1)
	return s.Tracker.RegisterRequest(ctx, id)
}

// The standby is never read while it waits, so once promoted it has to
// account for all the time that passed in one step.
func TestRegisterRequestLeavesStandbyToCatchUpOnDecay(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.Pi, cfg.Pd, cfg.Lambda = .5, .25, math.Ln2 // Half-life of one second.
	cfg.IncludeStats = true
	clock := &manualClock{now: time.Unix(1_700_000_000, 0)}
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, clock, ticker)
	require.NoError(t, err)
	defer ft.Close()

	ft.rotationLock.Lock()
	standby := &countingStandby{Tracker: ft.secondaryStructure}
	ft.secondaryStructure = standby
	ft.rotationLock.Unlock()

	ctx, id := context.Background(), []byte("client")
	probability := func() float64 { return ft.RegisterRequest(ctx, id).ResultStats.FinalProbability }

	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	require.Equal(t, 1., probability())

	for quarter := 1; quarter <= 4; quarter++ {
		clock.Sleep(250 * time.Millisecond)
		require.InDelta(t, math.Pow(2, -float64(quarter)/4), probability(), 1e-12)
	}
	require.Zero(t, standby.registers.Load(), "RegisterRequest must not read the standby structure")

	ticker.tickAndWait(t, ft)
	require.Same(t, standby, ft.mainStructure)
	require.InDelta(t, .5, probability(), 1e-12, "promoted standby did not catch up on decay")
}

func TestReportOutcomeIgnoresUnknownOutcome(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.Pi, cfg.Pd, cfg.Lambda = .5, .25, 0
	cfg.IncludeStats = true
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, &manualClock{now: time.Unix(1_700_000_000, 0)}, ticker)
	require.NoError(t, err)
	defer ft.Close()

	ctx, id := context.Background(), []byte("client")
	probability := func() float64 { return ft.RegisterRequest(ctx, id).ResultStats.FinalProbability }

	for _, outcome := range []request.Outcome{-1, 2, 42} {
		require.NotNil(t, ft.ReportOutcome(ctx, id, outcome))
		require.Zero(t, probability(), "outcome %d must not penalize", outcome)
	}

	// The standby must not have been penalized either.
	ticker.tickAndWait(t, ft)
	require.Zero(t, probability())
}

// A read can be the only call that observed the latest time. Promotion must
// not hand decisions to a standby that still believes an earlier one.
func TestPromotionKeepsDecayObservedBeforeClockRewind(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.Pi, cfg.Pd, cfg.Lambda = .5, .25, math.Ln2 // Half-life of one second.
	cfg.IncludeStats = true
	clock := &manualClock{now: time.Unix(1_700_000_000, 0)} // Wall time only, so it can rewind.
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, clock, ticker)
	require.NoError(t, err)
	defer ft.Close()

	ctx, id := context.Background(), []byte("client")
	probability := func() float64 { return ft.RegisterRequest(ctx, id).ResultStats.FinalProbability }

	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	require.Equal(t, 1., probability())

	clock.Sleep(time.Second)
	require.InDelta(t, .5, probability(), 1e-12)

	clock.Sleep(-time.Second) // Rewind, then rotate without another outcome.
	ticker.tickAndWait(t, ft)
	require.InDelta(t, .5, probability(), 1e-12, "promotion restored a penalty that had already decayed")

	// The repeated second was already counted; only time past it decays.
	clock.Sleep(time.Second)
	require.InDelta(t, .5, probability(), 1e-12)
	clock.Sleep(time.Second)
	require.InDelta(t, .25, probability(), 1e-12)
}

// Rotation is the recovery path when decay is disabled. A blocked client
// reports nothing, so the standby built after its last failure never learns
// of it. Carrying the penalty into the standby instead would block the client
// for good, and would keep a wrongly penalized client penalized.
func TestBlockedClientRecoversAfterTwoRotationsWithoutDecay(t *testing.T) {
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.Pi, cfg.Pd, cfg.Lambda = .5, .25, 0
	cfg.IncludeStats = true
	clock := &manualClock{now: time.Unix(1_700_000_000, 0)}
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, clock, ticker)
	require.NoError(t, err)
	defer ft.Close()

	ctx, id := context.Background(), []byte("client")
	probability := func() float64 { return ft.RegisterRequest(ctx, id).ResultStats.FinalProbability }

	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	require.Equal(t, 1., probability())

	// Time alone never forgives without decay, however often the client asks.
	clock.Sleep(24 * time.Hour)
	require.Equal(t, 1., probability())

	// The standby saw both failures, so the first promotion keeps the block.
	ticker.tickAndWait(t, ft)
	require.Equal(t, 1., probability())

	// The standby built at that promotion saw nothing.
	ticker.tickAndWait(t, ft)
	require.Zero(t, probability(), "a blocked client must recover at the second rotation")

	// A client that is still misbehaving is blocked again.
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	require.Equal(t, 1., probability())
}
