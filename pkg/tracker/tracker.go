package tracker

import (
	"context"
	"hash/maphash"
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

	// A stable stripe per client orders its updates without retaining client IDs.
	outcomeSeed  maphash.Seed
	outcomeLocks [64]sync.Mutex

	ticker utils.ITicker

	// Rotation lock to ensure that we don't rotate while updating the structures
	// The act of updating is a "read" in this case since multiple updates can happen
	// concurrently, but none can happen while we are rotating so that's a write.
	rotationLock   sync.RWMutex
	rotationStatus RotationStatus // protected by rotationLock
	stopRotation   chan struct{}
	rotationDone   chan struct{}
	closeOnce      sync.Once
}

// RotationStatus describes background rotation attempts for one tracker.
// The initial construction is not counted as a rotation attempt.
type RotationStatus struct {
	// Rotations counts successful promotions over the lifetime of the tracker.
	Rotations uint64
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
// on failure the caller retains ownership. Like time.Ticker's channel accessor,
// ticker.C must return without waiting for a tick or for another goroutine.
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

		outcomeSeed: maphash.MakeSeed(),
		ticker:      ticker,

		stopRotation: stopRotation,
		rotationDone: make(chan struct{}),
	}

	// Start a periodic task to rotate underlying structures to keep
	// changing the hash seeds so we don't continue punishing the same
	// innocent workloads repeatedly in the worst case of a false positive.
	go func() {
		defer close(ft.rotationDone)
		ticks := ticker.C()
		for {
			select {
			case <-stopRotation:
				return
			case _, ok := <-ticks:
				if !ok {
					return
				}
				// If both cases were ready, don't start another allocation.
				select {
				case <-stopRotation:
					return
				default:
				}
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
				ft.rotationStatus.Rotations++
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

func (ft *FairnessTracker) outcomeLock(clientIdentifier []byte) *sync.Mutex {
	return &ft.outcomeLocks[maphash.Bytes(ft.outcomeSeed, clientIdentifier)%uint64(len(ft.outcomeLocks))]
}

// ReportOutcome applies an outcome to the active and standby structures. For
// the same client, overlapping reports may run in either order, but both
// structures see the same order. Rotation never splits a single report.
func (ft *FairnessTracker) ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome request.Outcome) *request.ReportOutcomeResult {
	// Wait for this client's turn before pinning a generation pair.
	mu := ft.outcomeLock(clientIdentifier)
	mu.Lock()
	defer mu.Unlock()

	ft.rotationLock.RLock()
	defer ft.rotationLock.RUnlock()

	resp := ft.mainStructure.ReportOutcome(ctx, clientIdentifier, outcome)

	// To keep the bad workloads data "warm" in the rotated structure, we will update both
	ft.secondaryStructure.ReportOutcome(ctx, clientIdentifier, outcome)

	return resp
}

// Close stops background rotation and releases the ticker. It is safe to call
// repeatedly or concurrently. Every caller waits for the rotation worker to
// exit; an in-flight rotation may finish before Close returns. RegisterRequest
// and ReportOutcome remain usable with the final active and standby structures.
// Do not call Close from inside a tracker callback: rotation may be waiting
// for that callback to return.
func (ft *FairnessTracker) Close() {
	ft.closeOnce.Do(func() {
		close(ft.stopRotation)
		<-ft.rotationDone
		ft.ticker.Stop()
	})
}
