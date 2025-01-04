package tracker

import (
	"context"
	"log"
	"sync"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/data"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/utils"
)

// The main public facing object from this library
// Tracks the clients/flows from an application for fairness of their resource usage
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
	rotationLock sync.RWMutex
	stopRotation chan struct{}
}

// Allows passing an external ticket for simulations
func NewFairnessTrackerWithClockAndTicker(trackerConfig *config.FairnessTrackerConfig, clock utils.IClock, ticker utils.ITicker) (*FairnessTracker, error) {
	st1, err := data.NewStructureWithClock(trackerConfig, 1, trackerConfig.IncludeStats, clock)
	if err != nil {
		return nil, NewFairnessTrackerError(err, "Failed to create a structure")
	}

	st2, err := data.NewStructureWithClock(trackerConfig, 2, trackerConfig.IncludeStats, clock)
	if err != nil {
		return nil, NewFairnessTrackerError(err, "Failed to create a structure")
	}

	stopRotation := make(chan struct{})
	ft := &FairnessTracker{
		trackerConfig:      trackerConfig,
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
				s, err := data.NewStructureWithClock(trackerConfig, ft.structureIDCounter, trackerConfig.IncludeStats, clock)
				if err != nil {
					// TODO: While this should never happen, think if we want to handle this more gracefully
					log.Fatalf("Failed to create a structure during rotation")
				}
				ft.structureIDCounter++

				ft.rotationLock.Lock()
				ft.mainStructure = ft.secondaryStructure
				ft.secondaryStructure = s
				ft.rotationLock.Unlock()
			}
		}
	}()

	return ft, nil
}

func NewFairnessTracker(trackerConfig *config.FairnessTrackerConfig) (*FairnessTracker, error) {
	clk := utils.NewRealClock()
	ticker := utils.NewRealTicker(trackerConfig.RotationFrequency)
	return NewFairnessTrackerWithClockAndTicker(trackerConfig, clk, ticker)
}

func (ft *FairnessTracker) RegisterRequest(ctx context.Context, clientIdentifier []byte) *request.RegisterRequestResult {
	// We must take the rotation lock to avoid rotation while updating the structures
	ft.rotationLock.RLock()
	defer ft.rotationLock.RUnlock()

	resp := ft.mainStructure.RegisterRequest(ctx, clientIdentifier)

	// To keep the bad workloads data "warm" in the rotated structure, we will update both
	ft.secondaryStructure.RegisterRequest(ctx, clientIdentifier)

	return resp
}

func (ft *FairnessTracker) ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome request.Outcome) *request.ReportOutcomeResult {
	// We must take the rotation lock to avoid rotation while updating the structures
	ft.rotationLock.RLock()
	defer ft.rotationLock.RUnlock()

	resp := ft.mainStructure.ReportOutcome(ctx, clientIdentifier, outcome)

	// To keep the bad workloads data "warm" in the rotated structure, we will update both
	ft.secondaryStructure.ReportOutcome(ctx, clientIdentifier, outcome)

	return resp
}

func (ft *FairnessTracker) Close() {
	close(ft.stopRotation)
	ft.ticker.Stop()
}
