// Package simticker drives real tracker rotations under a simulation clock.
package simticker

import (
	"fmt"
	"time"

	"github.com/satmihir/fair/v2/pkg/tracker"
)

// Ticker is a single-driver ticker: only Rotate can trigger a tick.
type Ticker struct {
	ticks chan time.Time
}

// New constructs a ticker that stays idle until explicitly advanced.
func New() *Ticker { return &Ticker{ticks: make(chan time.Time)} }

// C supplies ticks to the tracker's background worker.
func (t *Ticker) C() <-chan time.Time { return t.ticks }

// Stop has no resources to release; Close stops the worker itself.
func (*Ticker) Stop() {}

// Rotate sends one tick and waits for publication before simulation time advances.
// The wall-clock timeout only detects a stalled worker; it never advances simulated time.
func (t *Ticker) Rotate(ft *tracker.FairnessTracker, at time.Time) error {
	before := ft.RotationStatus()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	select {
	case t.ticks <- at:
	case <-deadline.C:
		return fmt.Errorf("rotation worker did not accept tick")
	}
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		after := ft.RotationStatus()
		if after.Failures != before.Failures {
			return fmt.Errorf("rotation failed: %w", after.LastError)
		}
		if after.Rotations == before.Rotations+1 {
			return nil
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			return fmt.Errorf("rotation was not published after tick")
		}
	}
}
