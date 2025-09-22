package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClock(t *testing.T) {
	var clk IClock = NewRealClock()
	t1 := time.Now()
	t2 := clk.Now()

	assert.True(t, t2.Compare(t1) >= 0)

	clk.Sleep(10 * time.Millisecond)

	assert.True(t, clk.Now().Sub(t2) >= 10*time.Millisecond)
}

func TestTicker(t *testing.T) {
	var ticker ITicker = NewRealTicker(10 * time.Millisecond)
	var found bool

	select {
	case <-ticker.C():
		found = true
	case <-time.After(100 * time.Millisecond):
	}

	assert.True(t, found)

	ticker = NewRealTicker(50 * time.Millisecond)
	instance := 0
	timeoutChan := time.After(100 * time.Millisecond)
	ticker.Reset(10 * time.Millisecond)
loop:
	for {
		select {
		case <-ticker.C():
			instance++
		case <-timeoutChan:
			break loop
		}
	}

	assert.True(t, instance > 3)
}
