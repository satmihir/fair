package tracker

// Counters are cumulative totals since the tracker was built. Each field is
// read separately, so a snapshot taken under load may be off by the requests
// in flight; differences between two snapshots give rates.
type Counters struct {
	// Registered counts RegisterRequest calls.
	Registered uint64
	// Throttled counts the RegisterRequest calls that returned ShouldThrottle.
	Throttled uint64
	// Successes and Failures count the outcomes applied by ReportOutcome.
	Successes uint64
	Failures  uint64
	// IgnoredOutcomes counts ReportOutcome calls with an unknown outcome.
	// Anything above zero is a bug in the caller.
	IgnoredOutcomes uint64
}

// Counters returns the tracker's cumulative totals. It is cheap enough to call
// on every metrics scrape.
func (ft *FairnessTracker) Counters() Counters {
	return Counters{
		Registered:      ft.registered.Load(),
		Throttled:       ft.throttled.Load(),
		Successes:       ft.successes.Load(),
		Failures:        ft.failures.Load(),
		IgnoredOutcomes: ft.ignoredOutcomes.Load(),
	}
}

// PenalizedThreshold is the throttling probability at or above which
// Saturation counts a bucket as penalized: a client whose every bucket is at
// this level has at least 1 request in 100 throttled.
const PenalizedThreshold = 0.01

// Saturation describes how much of the active structure is carrying
// penalties. Clients share buckets, so as more buckets are penalized, more
// well-behaved clients are throttled for someone else's failures. A structure
// fills up when there are more misbehaving clients than it was sized for, or
// when one caller spreads failures across many identifiers.
type Saturation struct {
	// Levels and BucketsPerLevel are the structure's dimensions (L and M).
	Levels          int
	BucketsPerLevel int
	// PenalizedByLevel holds, for each level, the number of buckets at or
	// above PenalizedThreshold after decay.
	PenalizedByLevel []int
	// MeanProbability is the average throttling probability over all buckets:
	// 0 for an empty structure, 1 when every bucket blocks every request.
	MeanProbability float64
}

// PenalizedFraction is the share of all buckets that are penalized.
func (s Saturation) PenalizedFraction() float64 {
	buckets := s.Levels * s.BucketsPerLevel
	if buckets == 0 {
		return 0
	}
	penalized := 0
	for _, n := range s.PenalizedByLevel {
		penalized += n
	}
	return float64(penalized) / float64(buckets)
}

// SharedPenaltyChance estimates the chance that every bucket selected for a
// client with no failures of its own is at or above PenalizedThreshold. It is
// the product of each level's penalized share.
//
// It measures how much of the structure is occupied, not how often such a
// client is rejected. The rejection probability is whatever the buckets hold:
// with a single bucket at 0.01, this returns 1, yet a client sharing that
// bucket is throttled 1 time in 100. Alert on it rising: the tuner sizes a
// structure to keep it at or below 0.0001.
func (s Saturation) SharedPenaltyChance() float64 {
	if s.BucketsPerLevel == 0 || len(s.PenalizedByLevel) == 0 {
		return 0
	}
	chance := 1.
	for _, n := range s.PenalizedByLevel {
		chance *= float64(n) / float64(s.BucketsPerLevel)
	}
	return chance
}

// occupancyReporter is implemented by structures that can report saturation.
type occupancyReporter interface {
	PenaltyOccupancy(threshold float64) (penalizedByLevel []int, meanProbability float64)
}

// Saturation scans the active structure and reports how full it is. The scan
// reads every bucket, so its cost grows with L×M: call it from a metrics
// scrape, not per request. Requests run alongside it. A rotation that comes
// due during the scan waits for it to finish, because rotation closes the
// structure it retires, and requests that arrive while that rotation is
// waiting wait with it.
func (ft *FairnessTracker) Saturation() Saturation {
	ft.rotationLock.RLock()
	defer ft.rotationLock.RUnlock()
	active := ft.mainStructure

	report := Saturation{
		Levels:          int(ft.trackerConfig.L),
		BucketsPerLevel: int(ft.trackerConfig.M),
	}
	if reporter, ok := active.(occupancyReporter); ok {
		report.PenalizedByLevel, report.MeanProbability = reporter.PenaltyOccupancy(PenalizedThreshold)
	} else {
		report.PenalizedByLevel = make([]int, report.Levels)
	}
	return report
}
