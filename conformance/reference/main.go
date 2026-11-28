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

	"github.com/satmihir/fair/v2/internal/simticker"
	"github.com/satmihir/fair/v2/pkg/config"
	"github.com/satmihir/fair/v2/pkg/request"
	"github.com/satmihir/fair/v2/pkg/tracker"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time      { return time.Unix(1, 0) }
func (fixedClock) Sleep(time.Duration) {}

func run(in io.Reader, out io.Writer) error {
	cfg := &config.FairnessTrackerConfig{
		L: 3, M: 64, Pi: 0.25, Pd: 0.125, Lambda: 0.1,
		IncludeStats:             true,
		FinalProbabilityFunction: config.MinFinalProbabilityFunction,
	}
	clock := fixedClock{}
	ticker := simticker.New()
	ft, err := tracker.NewFairnessTrackerWithClockAndTicker(cfg, clock, ticker)
	if err != nil {
		return err
	}
	defer ft.Close()

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
			if err := ticker.Rotate(ft, clock.Now()); err != nil {
				return fmt.Errorf("rotate: %w", err)
			}
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
