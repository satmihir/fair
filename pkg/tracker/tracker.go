package tracker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/data"
	"github.com/satmihir/fair/pkg/request"
)

// The main public facing object from this library
// Tracks the clients/flows from an application for fairness of their resource usage
type FairnessTracker struct {
	trackerConfig *config.FairnessTrackerConfig

	// A counter to uniquely identify a structure
	structureIdCtr uint64

	mainStructure      request.Tracker
	secondaryStructure request.Tracker

	rotationLock *sync.Mutex
	stopRotation chan bool
}

func NewFairnessTracker(trackerConfig *config.FairnessTrackerConfig) (*FairnessTracker, error) {
	st1, err := data.NewStructure(trackerConfig, 1)
	if err != nil {
		return nil, NewFairnessTrackerError(err, "Failed to create a structure")
	}

	st2, err := data.NewStructure(trackerConfig, 2)
	if err != nil {
		return nil, NewFairnessTrackerError(err, "Failed to create a structure")
	}

	stopRotation := make(chan bool)
	ft := &FairnessTracker{
		trackerConfig:  trackerConfig,
		structureIdCtr: 3,

		mainStructure:      st1,
		secondaryStructure: st2,

		rotationLock: &sync.Mutex{},
		stopRotation: stopRotation,
	}

	// Start a periodic task to rotate underlying structures to keep
	// changing the hash seeds so we don't continue punishing the same
	// innocent workloads repeatedly in the worst case of a false positive.
	tikr := time.NewTicker(trackerConfig.RotationFrequency)
	go func() {
		for {
			select {
			case <-stopRotation:
				return
			case <-tikr.C:
				ft.rotationLock.Lock()
				defer ft.rotationLock.Unlock()

				s, err := data.NewStructure(trackerConfig, ft.structureIdCtr)
				ft.structureIdCtr++

				if err != nil {
					// TODO: While this should never happen, think if we want to handle this more gracefully
					log.Fatalf("Failed to create a structure during rotation")
				}

				ft.mainStructure = ft.secondaryStructure
				ft.secondaryStructure = s
			}
		}
	}()

	return ft, nil
}

func (ft *FairnessTracker) RegisterRequest(ctx context.Context, clientIdentifier []byte) (*request.RegisterRequestResult, error) {
	// We must take the rotation lock to avoid rotation while updating the structures
	ft.rotationLock.Lock()
	defer ft.rotationLock.Unlock()

	resp, err := ft.mainStructure.RegisterRequest(ctx, clientIdentifier)
	if err != nil {
		return nil, NewFairnessTrackerError(err, "Failed updating the primary structure")
	}

	// To keep the bad workloads data "warm" in the rotated structure, we will update both
	if _, err := ft.secondaryStructure.RegisterRequest(ctx, clientIdentifier); err != nil {
		// TODO: We don't really have to fail here perhaps but I cannot think any reason this will actually fail
		return nil, NewFairnessTrackerError(err, "Failed updating the secondary structure")
	}

	return resp, nil
}

func (ft *FairnessTracker) ReportOutcome(ctx context.Context, clientIdentifier []byte, outcome request.Outcome) (*request.ReportOutcomeResult, error) {
	// We must take the rotation lock to avoid rotation while updating the structures
	ft.rotationLock.Lock()
	defer ft.rotationLock.Unlock()

	resp, err := ft.mainStructure.ReportOutcome(ctx, clientIdentifier, outcome)
	if err != nil {
		return nil, NewFairnessTrackerError(err, "Failed updating the primary structure")
	}

	// To keep the bad workloads data "warm" in the rotated structure, we will update both
	if _, err := ft.secondaryStructure.ReportOutcome(ctx, clientIdentifier, outcome); err != nil {
		return nil, NewFairnessTrackerError(err, "Failed updating the secondary structure")
	}

	return resp, nil
}

func (ft *FairnessTracker) Close() {
	close(ft.stopRotation)
}
