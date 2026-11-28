package tracker

import (
	"context"
	"sync"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/data"
	"github.com/satmihir/fair/pkg/logger"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/utils"
)

// FairnessTracker is the main entry point for applications. It keeps track of
// client flows and determines when a request should be throttled to maintain
// fairness.
type FairnessTracker struct {
	trackerConfig *config.FairnessTrackerConfig

	// A counter to uniquely identify a structure
	structureIDCounter uint64

	mainStructure      request.Tracker
	secondaryStructure request.Tracker

	ticker utils.ITicker

	// Rotation lock to ensure that we don't rotate while updating the structures
	// The act of updating is a "read" in this case since multiple updates can happen
	// concurrently, but none can happen while we are rotating so that's a write.
	rotationLock   sync.RWMutex
	rotationStatus RotationStatus // protected by rotationLock
	stopRotation   chan struct{}
}

// RotationStatus describes background rotation attempts for one tracker.
// The initial construction is not counted as a rotation attempt.
type RotationStatus struct {
	// Failures counts failed attempts over the lifetime of the tracker.
	Failures uint64
	// LastError is the most recent rotation error, cleared on successful rotation.
	LastError error
}

// RotationStatus returns a consistent snapshot, even when logging is disabled.
func (ft *FairnessTracker) RotationStatus() RotationStatus {
	ft.rotationLock.RLock()
	defer ft.rotationLock.RUnlock()
	return ft.rotationStatus
}

var newTrackerStructureWithClock = func(
	trackerConfig *config.FairnessTrackerConfig,
	id uint64,
	includeStats bool,
	clock utils.IClock,
) (request.Tracker, error) {
	return data.NewStructureWithClock(trackerConfig, id, includeStats, clock)
}

// NewFairnessTrackerWithClockAndTicker creates a FairnessTracker using the
// provided clock and ticker. It is primarily used for tests and simulations
// where time needs to be controlled. On success the tracker owns the ticker;
// on failure the caller retains ownership.
func NewFairnessTrackerWithClockAndTicker(trackerConfig *config.FairnessTrackerConfig, clock utils.IClock, ticker utils.ITicker) (*FairnessTracker, error) {
	if trackerConfig == nil {
		return nil, NewFairnessTrackerError(nil, "trackerConfig must not be nil")
	}
	if utils.IsNil(clock) {
		return nil, NewFairnessTrackerError(nil, "clock must not be nil")
	}
	if utils.IsNil(ticker) {
		return nil, NewFairnessTrackerError(nil, "ticker must not be nil")
	}
	return newFairnessTracker(*trackerConfig, clock, ticker)
}

// Take the config by value so future rotations use the same settings as the initial pair.
func newFairnessTracker(cfg config.FairnessTrackerConfig, clock utils.IClock, ticker utils.ITicker) (*FairnessTracker, error) {
	newStructure := newTrackerStructureWithClock
	st1, err := newStructure(&cfg, 1, cfg.IncludeStats, clock)
	if err != nil {
		logger.Printf("error in creating first tracker : %v", err)
		return nil, NewFairnessTrackerError(err, "Failed to create a structure")
	}

	st2, err := newStructure(&cfg, 2, cfg.IncludeStats, clock)
	if err != nil {
		st1.Close()
		logger.Printf("error in creating second tracker : %v", err)
		return nil, NewFairnessTrackerError(err, "Failed to create a structure")
	}

	stopRotation := make(chan struct{})
	ft := &FairnessTracker{
		trackerConfig:      &cfg,
		structureIDCounter: 3,

		mainStructure:      st1,
		secondaryStructure: st2,

		ticker: ticker,

		rotationLock: sync.RWMutex{},
		stopRotation: stopRotation,
	}

	// Start a periodic task to rotate underlying structures to keep
	// changing the hash seeds so we don't continue punishing the same
	// innocent workloads repeatedly in the worst case of a false positive.
	go func() {
		for {
			select {
			case <-stopRotation:
				return
			case <-ticker.C():
				s, err := newStructure(&cfg, ft.structureIDCounter, cfg.IncludeStats, clock)
				if err != nil {
					ft.rotationLock.Lock()
					ft.rotationStatus.Failures++
					ft.rotationStatus.LastError = err
					ft.rotationLock.Unlock()
					logger.Printf("rotation failed; retaining current structures until the next tick: %v", err)
					continue
				}
				ft.structureIDCounter++

				ft.rotationLock.Lock()
				ft.mainStructure = ft.secondaryStructure
				ft.secondaryStructure = s
				ft.rotationStatus.LastError = nil
				ft.rotationLock.Unlock()
			}
		}
	}()
	return ft, nil
}

// NewFairnessTracker creates a FairnessTracker using the real system clock and
// ticker.
func NewFairnessTracker(trackerConfig *config.FairnessTrackerConfig) (*FairnessTracker, error) {
	if trackerConfig == nil {
		return nil, NewFairnessTrackerError(nil, "Configuration cannot be nil")
	}
	cfg := *trackerConfig
	if cfg.RotationFrequency <= 0 {
		return nil, NewFairnessTrackerError(nil, "rotation frequency must be positive")
	}
	if err := cfg.ValidateStructure(); err != nil {
		return nil, NewFairnessTrackerError(err, "invalid tracker configuration")
	}
	ticker := utils.NewRealTicker(cfg.RotationFrequency)
	ft, err := newFairnessTracker(cfg, utils.NewRealClock(), ticker)
	if err != nil {
		ticker.Stop()
		return nil, err
	}
	return ft, nil
}

// RegisterRequest records an incoming request and returns whether it should be
// throttled.
func (ft *FairnessTracker) RegisterRequest(ctx context.Context, clientIdentifier []byte) *request.RegisterRequestResult {
	// We must take the rotation lock to avoid rotation while updating the structures
	ft.rotationLock.RLock()
	defer ft.rotationLock.RUnlock()

	resp := ft.mainStructure.RegisterRequest(ctx, clientIdentifier)

	// To keep the bad workloads data "warm" in the rotated structure, we will update both
	ft.secondaryStructure.RegisterRequest(ctx, clientIdentifier)

	return resp
}

// ReportOutcome updates the trackers with the outcome of the request from the
// given client identifier.
func (ft *FairnessTracker) ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome request.Outcome) *request.ReportOutcomeResult {
	// We must take the rotation lock to avoid rotation while updating the structures
	ft.rotationLock.RLock()
	defer ft.rotationLock.RUnlock()

	resp := ft.mainStructure.ReportOutcome(ctx, clientIdentifier, outcome)

	// To keep the bad workloads data "warm" in the rotated structure, we will update both
	ft.secondaryStructure.ReportOutcome(ctx, clientIdentifier, outcome)

	return resp
}

// Close stops the background rotation goroutine and releases ticker resources.
func (ft *FairnessTracker) Close() {
	close(ft.stopRotation)
	ft.ticker.Stop()
}
