// Command reference serves the FAIR conformance protocol on standard input and output.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/tracker"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time      { return time.Unix(1, 0) }
func (fixedClock) Sleep(time.Duration) {}

type manualTicker struct {
	ticks chan time.Time
	ready chan struct{}
}

func (t manualTicker) C() <-chan time.Time {
	t.ready <- struct{}{}
	return t.ticks
}

func (manualTicker) Stop() {}

// The tracker calls C again only after installing the rotated structures.
func (t manualTicker) rotate() {
	t.ticks <- fixedClock{}.Now()
	<-t.ready
}

func run(in io.Reader, out io.Writer) error {
	cfg := &config.FairnessTrackerConfig{
		L: 3, M: 64, Pi: 0.25, Pd: 0.125, Lambda: 0.1,
		IncludeStats:             true,
		FinalProbabilityFunction: config.MinFinalProbabilityFunction,
	}
	ticker := manualTicker{ticks: make(chan time.Time), ready: make(chan struct{})}
	ft, err := tracker.NewFairnessTrackerWithClockAndTicker(cfg, fixedClock{}, ticker)
	if err != nil {
		return err
	}
	defer ft.Close()
	<-ticker.ready // Consume the initial C call; later calls acknowledge rotations.

	ctx := context.Background()
	dec, enc := json.NewDecoder(in), json.NewEncoder(out)
	for {
		var cmd struct {
			Op     string `json:"op"`
			Client string `json:"client"`
		}
		if err := dec.Decode(&cmd); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		var resp struct {
			Probability *float64 `json:"probability,omitempty"`
		}
		client := []byte(cmd.Client)
		switch cmd.Op {
		case "register":
			res := ft.RegisterRequest(ctx, client)
			resp.Probability = &res.ResultStats.FinalProbability
		case "failure":
			ft.ReportOutcome(ctx, client, request.OutcomeFailure)
		case "success":
			ft.ReportOutcome(ctx, client, request.OutcomeSuccess)
		case "rotate":
			ticker.rotate()
		default:
			return fmt.Errorf("unknown operation %q", cmd.Op)
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
