package utils

import "time"

// IClock abstracts time functions so that the tracker can be tested with custom
// clocks.
// It can be implemented using a mock clock to run simulations.
type IClock interface {
	// Get the current time
	Now() time.Time
	// Sleep for the given duration
	Sleep(duration time.Duration)
}

// Clock implements IClock using the real time package.
type Clock struct{}

// NewRealClock returns a Clock that uses the system clock.
func NewRealClock() *Clock {
	return &Clock{}
}

// Now returns the current time.
func (c *Clock) Now() time.Time {
	return time.Now()
}

// Sleep pauses for the specified duration.
func (c *Clock) Sleep(duration time.Duration) {
	time.Sleep(duration)
}

// ITicker abstracts a time.Ticker so that time can be controlled in tests.
type ITicker interface {
	C() <-chan time.Time
	Stop()
	Reset(time.Duration)
}

// Ticker wraps time.Ticker to satisfy the ITicker interface.
type Ticker struct {
	ticker *time.Ticker
}

// NewRealTicker creates a ticker that ticks at the specified duration.
func NewRealTicker(duration time.Duration) *Ticker {
	return &Ticker{
		ticker: time.NewTicker(duration),
	}
}

// C returns the underlying ticker's channel.
func (t *Ticker) C() <-chan time.Time {
	return t.ticker.C
}

// Stop stops the ticker.
func (t *Ticker) Stop() {
	t.ticker.Stop()
}

// Reset stops ticker and reset to specified period
// Next ticker arries after new period elapses
func (t *Ticker) Reset(duration time.Duration) {
	t.ticker.Reset(duration)
}
