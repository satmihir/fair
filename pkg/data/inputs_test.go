package data

import (
	"context"
	"math"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/satmihir/fair/v2/pkg/request"
)

func TestReportOutcomeIgnoresUnknownOutcome(t *testing.T) {
	cfg := validStructureConfig()
	cfg.Pi, cfg.Pd = .5, .25
	s, err := NewStructure(withStats(&cfg), 1)
	require.NoError(t, err)
	ctx, id := context.Background(), []byte("client")
	probability := func() float64 { return s.RegisterRequest(ctx, id).ResultStats.FinalProbability }

	for _, outcome := range []request.Outcome{-1, 2, 42} {
		s.ReportOutcome(ctx, id, outcome)
		require.Zero(t, probability(), "outcome %d must not penalize", outcome)
	}

	s.ReportOutcome(ctx, id, request.OutcomeFailure)
	s.ReportOutcome(ctx, id, request.Outcome(42))
	require.Equal(t, .5, probability(), "an unknown outcome must not reward either")
}

func TestFinalProbabilityIsSanitized(t *testing.T) {
	for name, tc := range map[string]struct {
		returned float64
		want     float64
	}{
		"NaN falls back to the minimum bucket": {math.NaN(), 1},
		"above one":                            {1.5, 1},
		"positive infinity":                    {math.Inf(1), 1},
		"negative":                             {-.5, 0},
		"negative infinity":                    {math.Inf(-1), 0},
		"in range is untouched":                {.25, .25},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validStructureConfig() // Pi is 1: one failure saturates the buckets.
			cfg.FinalProbabilityFunction = func([]float64) float64 { return tc.returned }
			s, err := NewStructure(withStats(&cfg), 1)
			require.NoError(t, err)
			ctx, id := context.Background(), []byte("client")
			s.ReportOutcome(ctx, id, request.OutcomeFailure)

			res := s.RegisterRequest(ctx, id)
			require.Equal(t, tc.want, res.ResultStats.FinalProbability)
			if tc.want == 1 {
				require.True(t, res.ShouldThrottle, "a saturated flow must still be throttled")
			}
		})
	}
}

type instantClock struct{ now time.Time }

func (c *instantClock) Now() time.Time      { return c.now }
func (c *instantClock) Sleep(time.Duration) {}

// stepWallClock returns t with its wall reading moved by d and its monotonic
// reading untouched: what time.Now hands out after the system clock is
// stepped. The time package has no constructor for that, so this edits the
// unexported layout and reports false if the result does not behave that way.
func stepWallClock(t time.Time, d time.Duration) (time.Time, bool) {
	type layout struct {
		wall uint64 // 1 bit hasMonotonic, 33 bits seconds, 30 bits nanoseconds
		ext  int64  // monotonic reading
		loc  *time.Location
	}
	if unsafe.Sizeof(t) != unsafe.Sizeof(layout{}) {
		return t, false
	}
	stepped := t
	(*layout)(unsafe.Pointer(&stepped)).wall += uint64(d/time.Second) << 30
	ok := stepped.Sub(t) == 0 && stepped.UnixMilli()-t.UnixMilli() == d.Milliseconds()
	return stepped, ok
}

func TestDecayIgnoresWallClockStep(t *testing.T) {
	base := time.Now() // Carries a monotonic reading, like the real clock.
	afterStep, ok := stepWallClock(base.Add(time.Second), 10*time.Minute)
	if !ok {
		t.Skip("cannot build a stepped wall clock with this Go version's time.Time")
	}

	cfg := validStructureConfig()
	cfg.Lambda = math.Ln2 // Half-life of one second.
	clock := &instantClock{now: base}
	s, err := NewStructureWithClock(withStats(&cfg), 1, clock)
	require.NoError(t, err)
	ctx, id := context.Background(), []byte("client")
	s.ReportOutcome(ctx, id, request.OutcomeFailure)

	// One second really passed; the wall clock claims ten minutes and a second.
	clock.now = afterStep
	for _, p := range s.RegisterRequest(ctx, id).ResultStats.BucketProbabilities {
		require.InDelta(t, .5, p, 1e-12, "a wall-clock step must not count as elapsed time")
	}
}

// The callback owns the slice it is handed, so the fallback cannot rely on it.
func TestNaNFallbackSurvivesCallbackMutation(t *testing.T) {
	cfg := validStructureConfig() // Pi is 1: one failure saturates the buckets.
	cfg.FinalProbabilityFunction = func(buckets []float64) float64 {
		buckets[0] = math.NaN()
		return math.NaN()
	}
	s, err := NewStructure(withStats(&cfg), 1)
	require.NoError(t, err)
	ctx, id := context.Background(), []byte("client")
	s.ReportOutcome(ctx, id, request.OutcomeFailure)

	res := s.RegisterRequest(ctx, id)
	require.Equal(t, 1., res.ResultStats.FinalProbability)
	require.True(t, res.ShouldThrottle, "a saturated flow must still be throttled")
}

// A request is throttled when the draw is below the probability. The draw
// is in [0, 1), so probability 0 never throttles and probability 1 always does.
func TestDecisionSourceDrivesThrottling(t *testing.T) {
	cfg := validStructureConfig()
	cfg.Pi, cfg.Pd = .5, .25
	var draw float64
	cfg.DecisionSource = func() float64 { return draw }
	s, err := NewStructure(withStats(&cfg), 1)
	require.NoError(t, err)
	ctx := context.Background()

	fresh, half, saturated := []byte("fresh"), []byte("half"), []byte("saturated")
	s.ReportOutcome(ctx, half, request.OutcomeFailure)
	s.ReportOutcome(ctx, saturated, request.OutcomeFailure)
	s.ReportOutcome(ctx, saturated, request.OutcomeFailure)

	almostOne := math.Nextafter(1, 0) // The largest draw the source can return.
	for _, tc := range []struct {
		id          []byte
		probability float64
		draw        float64
		throttle    bool
	}{
		{fresh, 0, 0, false},
		{fresh, 0, .5, false},
		{half, .5, 0, true},
		{half, .5, .49, true},
		{half, .5, .5, false},
		{half, .5, .51, false},
		{half, .5, almostOne, false},
		{saturated, 1, 0, true},
		{saturated, 1, .5, true},
		{saturated, 1, almostOne, true},
	} {
		draw = tc.draw
		res := s.RegisterRequest(ctx, tc.id)
		require.Equalf(t, tc.probability, res.ResultStats.FinalProbability, "%s", tc.id)
		require.Equalf(t, tc.throttle, res.ShouldThrottle, "probability %g, draw %g", tc.probability, tc.draw)
	}
}

func TestDefaultDecisionSourceThrottlesInProportion(t *testing.T) {
	cfg := validStructureConfig()
	cfg.Pi, cfg.Pd = .5, .25
	require.Nil(t, cfg.DecisionSource)
	s, err := NewStructure(&cfg, 1)
	require.NoError(t, err)
	ctx, id := context.Background(), []byte("client")
	s.ReportOutcome(ctx, id, request.OutcomeFailure) // Probability 0.5.

	const requests = 20000
	throttled := 0
	for i := 0; i < requests; i++ {
		if s.RegisterRequest(ctx, id).ShouldThrottle {
			throttled++
		}
	}
	// Six standard deviations of a fair coin over 20,000 flips is about 424.
	require.InDelta(t, requests/2, throttled, 430)
}
