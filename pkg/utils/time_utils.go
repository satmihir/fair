package utils

import "time"

// The interface for a clock that's used inside the library.
// Can be implemented using a mock clock to run in a simulation mode.
type IClock interface {
	// Get the current time
	Now() time.Time
	// Sleep for the given duration
	Sleep(duration time.Duration)
}

// Implementation of a clock using real time
type Clock struct{}

func NewRealClock() *Clock {
	return &Clock{}
}

func (c *Clock) Now() time.Time {
	return time.Now()
}

func (c *Clock) Sleep(duration time.Duration) {
	time.Sleep(duration)
}

// Similar to IClock, a ticker interface to be able to mock
// time in a simulation setup
type ITicker interface {
	C() <-chan time.Time
	Stop()
}

// A real implementation of a Ticker
type Ticker struct {
	ticker *time.Ticker
}

func NewRealTicker(duration time.Duration) *Ticker {
	return &Ticker{
		ticker: time.NewTicker(duration),
	}
}

func (t *Ticker) C() <-chan time.Time {
	return t.ticker.C
}

func (t *Ticker) Stop() {
	t.ticker.Stop()
}
