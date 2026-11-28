package tracker_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/satmihir/fair/v2/pkg/request"
	"github.com/satmihir/fair/v2/pkg/tracker"
)

func ExampleFairnessTracker() {
	builder := tracker.NewFairnessTrackerBuilder()
	// For this example only: make a single contention failure block the next call.
	builder.SetPi(1)
	builder.SetLambda(0)
	trk, err := builder.Build()
	if err != nil {
		panic(err) // Cannot serve requests without the tracker.
	}
	defer trk.Close()

	errPoolFull := errors.New("pool is full")
	errThrottled := errors.New("throttled")
	acquire := func(ctx context.Context, clientID []byte, tryAcquire func() error) error {
		if trk.RegisterRequest(ctx, clientID).ShouldThrottle {
			return errThrottled // Do not attempt acquisition or report an outcome.
		}
		err := tryAcquire()
		switch {
		case err == nil:
			trk.ReportOutcome(ctx, clientID, request.OutcomeSuccess)
		case errors.Is(err, errPoolFull):
			trk.ReportOutcome(ctx, clientID, request.OutcomeFailure)
		} // Other errors do not establish contention.
		return err
	}

	ctx := context.Background()
	clientID := []byte("client-42")
	fmt.Println(acquire(ctx, clientID, func() error { return nil }))
	fmt.Println(acquire(ctx, clientID, func() error { return errPoolFull }))
	fmt.Println(acquire(ctx, clientID, func() error { return nil })) // Not forwarded.
	// Output:
	// <nil>
	// pool is full
	// throttled
}

func ExampleFairnessTrackerBuilder_SetIncludeStats() {
	builder := tracker.NewFairnessTrackerBuilder()
	builder.SetIncludeStats(true)
	builder.SetL(2)
	trk, err := builder.Build()
	if err != nil {
		panic(err)
	}
	defer trk.Close()

	result := trk.RegisterRequest(context.Background(), []byte("client-42"))
	if stats := result.ResultStats; stats != nil {
		fmt.Println("final probability:", stats.FinalProbability)
		fmt.Println("selected buckets:", len(stats.BucketIndexes))
		fmt.Println("bucket probabilities:", stats.BucketProbabilities)
	}
	// Output:
	// final probability: 0
	// selected buckets: 2
	// bucket probabilities: [0 0]
}
