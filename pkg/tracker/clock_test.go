package tracker

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHighWaterClockNeverMovesBackward(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	source := &manualClock{now: start}
	clock := newHighWaterClock(source)
	require.Equal(t, start, clock.Now())

	source.Sleep(1500*time.Millisecond + 700*time.Microsecond)
	high := start.Add(1500 * time.Millisecond) // Whole milliseconds only.
	require.Equal(t, high, clock.Now())

	source.Sleep(-time.Second)
	require.Equal(t, high, clock.Now(), "a rewind must not be visible")
	source.Sleep(-time.Hour)
	require.Equal(t, high, clock.Now(), "a time before the epoch must not be visible")

	source.Sleep(time.Hour + time.Second)
	require.Equal(t, high, clock.Now(), "catching up must not count the interval twice")
	source.Sleep(time.Millisecond)
	require.Equal(t, high.Add(time.Millisecond), clock.Now())
}

func TestHighWaterClockConcurrentReaders(t *testing.T) {
	source := &manualClock{now: time.Unix(1_700_000_000, 0)}
	clock := newHighWaterClock(source)

	var wg sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			last := clock.Now()
			for i := 0; i < 2000; i++ {
				// Alternate forward and backward steps under the readers.
				if i%3 == 0 {
					source.Sleep(-time.Millisecond)
				} else {
					source.Sleep(time.Millisecond)
				}
				now := clock.Now()
				if now.Before(last) {
					t.Errorf("time moved backward: %v after %v", now, last)
					return
				}
				last = now
			}
		}()
	}
	wg.Wait()
}
