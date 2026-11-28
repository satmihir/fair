package integration_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/v2/internal/simticker"
	"github.com/satmihir/fair/v2/pkg/config"
	"github.com/satmihir/fair/v2/pkg/request"
	"github.com/satmihir/fair/v2/pkg/tracker"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time        { return c.now }
func (c *clock) Sleep(d time.Duration) { c.now = c.now.Add(d) }

type counts struct{ served, failed, throttled int }

func TestEnforcedNoisyNeighbor(t *testing.T) {
	c := &clock{now: time.Unix(100, 0)}
	ticker := simticker.New()
	cfg := &config.FairnessTrackerConfig{
		L: 4, M: 128, Pi: 1, Pd: 0.25,
		// Disable decay to distinguish rotation-based recovery from aging.
		Lambda: 0, FinalProbabilityFunction: config.MinFinalProbabilityFunction,
		// Fixed placement keys and decisions, so every run is the same.
		HashKeySource:  rand.New(rand.NewSource(9)),
		DecisionSource: rand.New(rand.NewSource(10)).Float64,
	}
	trk, err := tracker.NewFairnessTrackerWithClockAndTicker(cfg, c, ticker)
	require.NoError(t, err)
	t.Cleanup(trk.Close)

	// Model a downstream API with a fixed-window quota of four writes/second.
	// Only forwarded calls consume quota or produce feedback for the tracker.
	const capacity = 4
	remaining := capacity
	ctx := context.Background()
	run := func(client string, offered int) counts {
		var n counts
		id := []byte(client)
		for i := 0; i < offered; i++ {
			if trk.RegisterRequest(ctx, id).ShouldThrottle {
				n.throttled++
				continue
			}
			if remaining == 0 {
				n.failed++
				trk.ReportOutcome(ctx, id, request.OutcomeFailure)
			} else {
				remaining--
				n.served++
				trk.ReportOutcome(ctx, id, request.OutcomeSuccess)
			}
		}
		return n
	}
	nextWindow := func() {
		c.Sleep(time.Second)
		remaining = capacity
	}

	// A batch client bursts just before the quota resets.
	require.Equal(t, counts{served: 1}, run("interactive-a", 1))
	require.Equal(t, counts{served: 1}, run("interactive-b", 1))
	require.Equal(t, counts{served: 2, failed: 1, throttled: 13}, run("batch", 16))

	// Batch retries arrive first in every subsequent window. Enforcing the
	// decision must protect both interactive clients, even immediately after
	// promotion of the warm standby. Load shedding must not idle the backend.
	for window := 1; window <= 8; window++ {
		nextWindow()
		if window == 5 {
			require.NoError(t, ticker.Rotate(trk, c.Now()))
		}
		require.Equal(t, counts{throttled: 16}, run("batch", 16), "window %d", window)
		for _, id := range []string{"interactive-a", "interactive-b"} {
			require.Equal(t, counts{served: 2}, run(id, 2), "%s in window %d", id, window)
		}
		require.Zero(t, remaining, "unused backend capacity in window %d", window)
	}

	// After another rotation, the original burst's failure is no longer in
	// either active window. A right-sized batch can use its share again.
	nextWindow()
	require.NoError(t, ticker.Rotate(trk, c.Now()))
	require.Equal(t, counts{served: 2}, run("batch", 2))
	require.Equal(t, counts{served: 1}, run("interactive-a", 1))
	require.Equal(t, counts{served: 1}, run("interactive-b", 1))
	require.Zero(t, remaining, "unused backend capacity after recovery")
}
