package tracker

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
)

func signalsTracker(t *testing.T, mutate func(*config.FairnessTrackerConfig)) (*FairnessTracker, *manualClock, *fakeTicker) {
	t.Helper()
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.L, cfg.M = 3, 1000
	cfg.Pi, cfg.Pd, cfg.Lambda = .5, .25, 0
	cfg.HashKeySource = rand.New(rand.NewSource(21)) // Fixed placement.
	if mutate != nil {
		mutate(cfg)
	}
	clock := &manualClock{now: time.Unix(1_700_000_000, 0)}
	ticker := newFakeTicker()
	ft, err := NewFairnessTrackerWithClockAndTicker(cfg, clock, ticker)
	require.NoError(t, err)
	t.Cleanup(ft.Close)
	return ft, clock, ticker
}

func TestCountersTrackRequestsAndOutcomes(t *testing.T) {
	ft, _, _ := signalsTracker(t, nil)
	require.Equal(t, Counters{}, ft.Counters())

	ctx := context.Background()
	good, bad := []byte("good"), []byte("bad")

	// A saturated client is always throttled; an unknown one never is.
	ft.ReportOutcome(ctx, bad, request.OutcomeFailure)
	ft.ReportOutcome(ctx, bad, request.OutcomeFailure)
	for i := 0; i < 7; i++ {
		require.True(t, ft.RegisterRequest(ctx, bad).ShouldThrottle)
	}
	for i := 0; i < 5; i++ {
		require.False(t, ft.RegisterRequest(ctx, good).ShouldThrottle)
		ft.ReportOutcome(ctx, good, request.OutcomeSuccess)
	}
	ft.ReportOutcome(ctx, good, request.Outcome(42))

	require.Equal(t, Counters{
		Registered:      12,
		Throttled:       7,
		Successes:       5,
		Failures:        2,
		IgnoredOutcomes: 1,
	}, ft.Counters())
}

func TestCountersAreSafeUnderConcurrency(t *testing.T) {
	ft, _, _ := signalsTracker(t, nil)
	ctx := context.Background()
	const workers, each = 8, 500

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			id := []byte(fmt.Sprintf("client-%d", w))
			for i := 0; i < each; i++ {
				ft.RegisterRequest(ctx, id)
				ft.ReportOutcome(ctx, id, request.OutcomeSuccess)
				_ = ft.Counters()
			}
		}(w)
	}
	wg.Wait()

	got := ft.Counters()
	require.Equal(t, uint64(workers*each), got.Registered)
	require.Equal(t, uint64(workers*each), got.Successes)
	require.Zero(t, got.Throttled)
}

func TestSaturationOfEmptyStructure(t *testing.T) {
	ft, _, _ := signalsTracker(t, nil)
	got := ft.Saturation()
	require.Equal(t, Saturation{Levels: 3, BucketsPerLevel: 1000, PenalizedByLevel: []int{0, 0, 0}}, got)
	require.Zero(t, got.PenalizedFraction())
	require.Zero(t, got.SharedPenaltyChance())
}

func TestSaturationCountsPenalizedBuckets(t *testing.T) {
	ft, _, _ := signalsTracker(t, func(cfg *config.FairnessTrackerConfig) { cfg.IncludeStats = true })
	ctx := context.Background()

	// Record which bucket each failing client uses at each level.
	const clients = 40
	used := []map[int]bool{{}, {}, {}}
	for i := 0; i < clients; i++ {
		id := []byte(fmt.Sprintf("bad-%d", i))
		ft.ReportOutcome(ctx, id, request.OutcomeFailure) // Each bucket rises by 0.5.
		for level, bucket := range ft.RegisterRequest(ctx, id).ResultStats.BucketIndexes {
			used[level][bucket] = true
		}
	}

	got := ft.Saturation()
	want := []int{len(used[0]), len(used[1]), len(used[2])}
	require.Equal(t, want, got.PenalizedByLevel)

	// Buckets shared by two clients hold 1.0, so the total mass is not simply
	// 0.5 per distinct bucket; bound it instead.
	penalized := want[0] + want[1] + want[2]
	require.InDelta(t, float64(penalized)/3000, got.PenalizedFraction(), 1e-12)
	require.GreaterOrEqual(t, got.MeanProbability, .5*float64(penalized)/3000)
	require.LessOrEqual(t, got.MeanProbability, .5*float64(3*clients)/3000+1e-12)

	chance := float64(want[0]) / 1000 * float64(want[1]) / 1000 * float64(want[2]) / 1000
	require.InDelta(t, chance, got.SharedPenaltyChance(), 1e-15)
}

func TestSaturationAppliesDecayWithoutChangingState(t *testing.T) {
	ft, clock, _ := signalsTracker(t, func(cfg *config.FairnessTrackerConfig) {
		cfg.Lambda = math.Ln2 // Half-life of one second.
		cfg.IncludeStats = true
	})
	ctx, id := context.Background(), []byte("bad")
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	require.Equal(t, []int{1, 1, 1}, ft.Saturation().PenalizedByLevel)
	require.InDelta(t, .5*3/3000, ft.Saturation().MeanProbability, 1e-12)

	clock.Sleep(time.Second)
	require.InDelta(t, .25*3/3000, ft.Saturation().MeanProbability, 1e-12)

	// 0.5 halves six times to fall below the 0.01 threshold.
	clock.Sleep(5 * time.Second)
	require.Equal(t, []int{0, 0, 0}, ft.Saturation().PenalizedByLevel)

	// Scanning repeatedly must not have decayed anything twice.
	require.InDelta(t, .5/64, ft.RegisterRequest(ctx, id).ResultStats.FinalProbability, 1e-12)
}

func TestSaturationFollowsTheActiveStructure(t *testing.T) {
	ft, _, ticker := signalsTracker(t, nil)
	ctx, id := context.Background(), []byte("bad")
	ft.ReportOutcome(ctx, id, request.OutcomeFailure)
	require.Equal(t, []int{1, 1, 1}, ft.Saturation().PenalizedByLevel)

	ticker.tickAndWait(t, ft) // The warm standby takes over.
	require.Equal(t, []int{1, 1, 1}, ft.Saturation().PenalizedByLevel)

	ticker.tickAndWait(t, ft) // The next standby never heard of the client.
	require.Equal(t, []int{0, 0, 0}, ft.Saturation().PenalizedByLevel)
}

func TestSaturationWithoutAReportingStructure(t *testing.T) {
	ft, _, _ := signalsTracker(t, nil)
	ft.rotationLock.Lock()
	ft.mainStructure = &fakeTracker{id: 99}
	ft.rotationLock.Unlock()

	got := ft.Saturation()
	require.Equal(t, []int{0, 0, 0}, got.PenalizedByLevel)
	require.Zero(t, got.MeanProbability)
}

func TestSaturationHelpersHandleZeroValue(t *testing.T) {
	var s Saturation
	require.Zero(t, s.PenalizedFraction())
	require.Zero(t, s.SharedPenaltyChance())
}
