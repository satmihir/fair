package tracker

import (
	"sync/atomic"
	"time"

	"github.com/satmihir/fair/v2/pkg/utils"
)

// highWaterClock gives every generation of a tracker the same view of time,
// and that view never moves backward. RegisterRequest reads only the active
// structure, so the standby would otherwise miss a time that only a read
// observed: if the underlying clock then rewound, promotion would bring back
// a penalty that had already decayed.
//
// Time is kept as whole milliseconds since the tracker's epoch, which is the
// resolution decay works at. The shared value is therefore written at most
// once per millisecond however many requests read it.
type highWaterClock struct {
	clock  utils.IClock
	epoch  time.Time
	latest atomic.Int64 // Milliseconds since epoch; never decreases.
}

func newHighWaterClock(clock utils.IClock) *highWaterClock {
	return &highWaterClock{clock: clock, epoch: clock.Now()}
}

// Now returns the latest time any caller has observed. Sub uses the monotonic
// reading when the underlying clock supplies one, and Add preserves it.
func (c *highWaterClock) Now() time.Time {
	now := c.clock.Now().Sub(c.epoch).Milliseconds()
	for {
		latest := c.latest.Load()
		if now <= latest {
			now = latest
			break
		}
		if c.latest.CompareAndSwap(latest, now) {
			break
		}
	}
	return c.epoch.Add(time.Duration(now) * time.Millisecond)
}

// Sleep defers to the underlying clock.
func (c *highWaterClock) Sleep(d time.Duration) {
	c.clock.Sleep(d)
}
