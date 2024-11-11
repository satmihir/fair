package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/tracker"
)

var errNoTokens = fmt.Errorf("no tokens left")

type TokenBucket struct {
	tokens                float64
	tokensPerSecond       float64
	lastUpdatedTimeMillis uint64

	lk *sync.Mutex
}

func NewTokenBucket(initialTokens uint32, tokensPerSecond float64) *TokenBucket {
	return &TokenBucket{
		tokens:                float64(initialTokens),
		tokensPerSecond:       tokensPerSecond,
		lastUpdatedTimeMillis: uint64(time.Now().UnixMilli()),
		lk:                    &sync.Mutex{},
	}
}

func (tb *TokenBucket) Take() error {
	tb.lk.Lock()
	defer tb.lk.Unlock()

	diff := (uint64(time.Now().UnixMilli()) - tb.lastUpdatedTimeMillis) / 1000
	tb.tokens += tb.tokensPerSecond * float64(diff)

	if tb.tokens >= 1 {
		tb.tokens--
		return nil
	}

	return errNoTokens
}

func TestIntegration(t *testing.T) {
	tb := NewTokenBucket(10, 1)

	ctx := context.Background()

	trkB := tracker.NewFairnessTrackerBuilder()
	trkB.SetRotationFrequency(1 * time.Second)

	trk, err := trkB.Build()
	assert.NoError(t, err)
	defer trk.Close()

	var twoHundred atomic.Uint32
	var fourTwentyNine atomic.Uint32
	var throttles atomic.Uint32

	wg := &sync.WaitGroup{}

	for i := 0; i < 10; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			client := fmt.Sprintf("cl-%d", i)

			for j := 0; j < 100; j++ {
				res, err := trk.RegisterRequest(ctx, []byte(client))
				assert.NoError(t, err)

				if res.ShouldThrottle {
					throttles.Add(1)
				}

				if err := tb.Take(); err != nil {
					fourTwentyNine.Add(1)
					_, err = trk.ReportOutcome(ctx, []byte(client), request.OutcomeFailure)
					assert.NoError(t, err)
				} else {
					twoHundred.Add(1)
					_, err = trk.ReportOutcome(ctx, []byte(client), request.OutcomeSuccess)
					assert.NoError(t, err)
				}

				time.Sleep(25 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	assert.Greater(t, int(twoHundred.Load()), 0)
	assert.Greater(t, int(fourTwentyNine.Load()), 0)
	assert.Greater(t, int(throttles.Load()), 0)
}
