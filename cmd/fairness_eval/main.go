package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/tracker"
)

const (
	defaultBucketsPerLevel          = 1000
	defaultBadRequestsBeforeBlocked = 25
)

type workloadClass struct {
	name          string
	clients       int
	ratePerSecond float64
	start         time.Duration
	end           time.Duration
}

type scenario struct {
	name              string
	description       string
	duration          time.Duration
	capacityPerSecond float64
	burstTokens       float64
	classes           []workloadClass
}

type client struct {
	id    string
	class string
}

type event struct {
	at          time.Duration
	clientIndex int
}

type runResult struct {
	mode                  string
	successes             []float64
	requests              []float64
	throttled             int
	resourceFailures      int
	totalSuccesses        int
	rawJain               float64
	entitlementJain       float64
	resourceUtilization   float64
	meanEntitlementRatio  float64
	worstEntitlementRatio float64
}

type fairConfigVariant struct {
	name                     string
	bucketsPerLevel          uint32
	badRequestsBeforeBlocked uint32
	lambda                   float64
	finalProbabilityFunction config.FinalProbabilityFunction
}

type simClock struct {
	base time.Time
	now  time.Duration
}

func (c *simClock) Now() time.Time {
	return c.base.Add(c.now)
}

func (c *simClock) Sleep(duration time.Duration) {
	c.now += duration
}

func (c *simClock) set(now time.Duration) {
	c.now = now
}

type stoppedTicker struct {
	ch chan time.Time
}

func newStoppedTicker() *stoppedTicker {
	return &stoppedTicker{ch: make(chan time.Time)}
}

func (t *stoppedTicker) C() <-chan time.Time {
	return t.ch
}

func (t *stoppedTicker) Stop() {
}

type tokenBucket struct {
	tokens          float64
	maxTokens       float64
	tokensPerSecond float64
	lastSeconds     float64
}

func newTokenBucket(tokensPerSecond, maxTokens float64) *tokenBucket {
	return &tokenBucket{
		tokens:          maxTokens,
		maxTokens:       maxTokens,
		tokensPerSecond: tokensPerSecond,
	}
}

func (tb *tokenBucket) take(at time.Duration) bool {
	nowSeconds := at.Seconds()
	elapsed := nowSeconds - tb.lastSeconds
	tb.tokens = math.Min(tb.maxTokens, tb.tokens+tb.tokensPerSecond*elapsed)
	tb.lastSeconds = nowSeconds

	if tb.tokens < 1 {
		return false
	}

	tb.tokens--
	return true
}

func main() {
	scenarios := []scenario{
		{
			name:              "api_noisy_neighbors",
			description:       "180 steady API clients at 0.5 rps plus 20 aggressive clients at 10 rps contend for 100 rps",
			duration:          5 * time.Minute,
			capacityPerSecond: 100,
			burstTokens:       100,
			classes: []workloadClass{
				{name: "steady", clients: 180, ratePerSecond: 0.5},
				{name: "aggressive", clients: 20, ratePerSecond: 10},
			},
		},
		{
			name:              "flash_crowd",
			description:       "500 equivalent clients arrive as a Poisson flash crowd against 300 rps",
			duration:          2 * time.Minute,
			capacityPerSecond: 300,
			burstTokens:       300,
			classes: []workloadClass{
				{name: "shopper", clients: 500, ratePerSecond: 1},
			},
		},
		{
			name:              "batch_surge",
			description:       "100 interactive clients run steadily while 10 batch clients surge for the middle 3 minutes",
			duration:          6 * time.Minute,
			capacityPerSecond: 120,
			burstTokens:       120,
			classes: []workloadClass{
				{name: "interactive", clients: 100, ratePerSecond: 1},
				{name: "batch", clients: 10, ratePerSecond: 30, start: 90 * time.Second, end: 270 * time.Second},
			},
		},
	}

	fmt.Println("Jain fairness experiment")
	fmt.Println("Primary score is Jain(successes / max-min-fair entitlement). Raw Jain(successes) and utilization are included as guardrails.")
	fmt.Println()

	for i, sc := range scenarios {
		seed := int64(1000 + i)
		clients, events := generateEvents(sc, seed)
		entitlements := maxMinEntitlements(requestCounts(len(clients), events), serviceCapacity(sc, len(events)))

		baseline, err := runScenario(sc, clients, events, entitlements, false, seed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "baseline failed for %s: %v\n", sc.name, err)
			os.Exit(1)
		}

		withFair, err := runScenario(sc, clients, events, entitlements, true, seed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fair failed for %s: %v\n", sc.name, err)
			os.Exit(1)
		}

		printScenario(sc, len(clients), len(events), baseline, withFair)

		if sc.name == "api_noisy_neighbors" {
			printAPINoisyNeighborsSweep(sc, clients, events, entitlements, baseline, seed)
		}
	}
}

func generateEvents(sc scenario, seed int64) ([]client, []event) {
	rng := rand.New(rand.NewSource(seed))
	clients := make([]client, 0)
	events := make([]event, 0)

	for _, class := range sc.classes {
		start := class.start
		end := class.end
		if end == 0 {
			end = sc.duration
		}

		for i := 0; i < class.clients; i++ {
			clientIndex := len(clients)
			clients = append(clients, client{
				id:    fmt.Sprintf("%s-%03d", class.name, i),
				class: class.name,
			})

			for at := start + nextArrival(rng, class.ratePerSecond); at < end; at += nextArrival(rng, class.ratePerSecond) {
				events = append(events, event{at: at, clientIndex: clientIndex})
			}
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].at == events[j].at {
			return events[i].clientIndex < events[j].clientIndex
		}
		return events[i].at < events[j].at
	})

	return clients, events
}

func nextArrival(rng *rand.Rand, ratePerSecond float64) time.Duration {
	if ratePerSecond <= 0 {
		return time.Hour
	}

	seconds := -math.Log(1-rng.Float64()) / ratePerSecond
	return time.Duration(seconds * float64(time.Second))
}

func runScenario(sc scenario, clients []client, events []event, entitlements []float64, useFair bool, seed int64) (runResult, error) {
	return runScenarioWithVariant(sc, clients, events, entitlements, useFair, seed, fairConfigVariant{
		name:                     "default",
		bucketsPerLevel:          defaultBucketsPerLevel,
		badRequestsBeforeBlocked: defaultBadRequestsBeforeBlocked,
		lambda:                   0.01,
		finalProbabilityFunction: config.MinFinalProbabilityFunction,
	})
}

func runScenarioWithVariant(
	sc scenario,
	clients []client,
	events []event,
	entitlements []float64,
	useFair bool,
	seed int64,
	variant fairConfigVariant,
) (runResult, error) {
	result := runResult{
		mode:      "without FAIR",
		successes: make([]float64, len(clients)),
		requests:  make([]float64, len(clients)),
	}
	tb := newTokenBucket(sc.capacityPerSecond, sc.burstTokens)

	var trk *tracker.FairnessTracker
	var clock *simClock
	if useFair {
		result.mode = "with FAIR"
		clock = &simClock{base: time.Unix(0, 0)}

		conf, err := config.GenerateTunedStructureConfig(
			uint32(len(clients)),
			variant.bucketsPerLevel,
			variant.badRequestsBeforeBlocked,
		)
		if err != nil {
			return runResult{}, err
		}
		conf.Lambda = variant.lambda
		conf.FinalProbabilityFunction = variant.finalProbabilityFunction
		conf.RotationFrequency = sc.duration + time.Minute

		//nolint:staticcheck // The tracker uses math/rand's package global; seed it so this experiment is reproducible.
		rand.Seed(seed)
		trackerInstance, err := tracker.NewFairnessTrackerWithClockAndTicker(conf, clock, newStoppedTicker())
		if err != nil {
			return runResult{}, err
		}
		defer trackerInstance.Close()
		trk = trackerInstance
	}

	ctx := context.Background()
	for _, ev := range events {
		cl := clients[ev.clientIndex]
		result.requests[ev.clientIndex]++

		if useFair {
			clock.set(ev.at)
			decision := trk.RegisterRequest(ctx, []byte(cl.id))
			if decision.ShouldThrottle {
				result.throttled++
				continue
			}
		}

		if tb.take(ev.at) {
			result.successes[ev.clientIndex]++
			result.totalSuccesses++
			if useFair {
				trk.ReportOutcome(ctx, []byte(cl.id), request.OutcomeSuccess)
			}
			continue
		}

		result.resourceFailures++
		if useFair {
			trk.ReportOutcome(ctx, []byte(cl.id), request.OutcomeFailure)
		}
	}

	result.rawJain = jain(result.successes)
	result.entitlementJain, result.meanEntitlementRatio, result.worstEntitlementRatio = entitlementJain(result.successes, entitlements)
	result.resourceUtilization = float64(result.totalSuccesses) / serviceCapacity(sc, len(events))
	return result, nil
}

func requestCounts(clientCount int, events []event) []float64 {
	counts := make([]float64, clientCount)
	for _, ev := range events {
		counts[ev.clientIndex]++
	}
	return counts
}

func printAPINoisyNeighborsSweep(
	sc scenario,
	clients []client,
	events []event,
	entitlements []float64,
	baseline runResult,
	seed int64,
) {
	variants := make([]fairConfigVariant, 0)
	for _, finalProbabilityFunction := range []struct {
		name string
		fn   config.FinalProbabilityFunction
	}{
		{name: "min", fn: config.MinFinalProbabilityFunction},
		{name: "mean", fn: config.MeanFinalProbabilityFunction},
	} {
		for _, tolerance := range []uint32{20, 25, 30, 35, 40, 45, 50, 60, 75, 100} {
			for _, lambda := range []float64{0, 0.0001, 0.0005, 0.001, 0.005, 0.01} {
				for _, bucketsPerLevel := range []uint32{100, 200, 300, 500, 750, 1000, 1500, 2000, 3000} {
					variants = append(variants, fairConfigVariant{
						name: fmt.Sprintf(
							"%s,tol=%d,lambda=%s,buckets=%d",
							finalProbabilityFunction.name,
							tolerance,
							formatFloat(lambda),
							bucketsPerLevel,
						),
						bucketsPerLevel:          bucketsPerLevel,
						badRequestsBeforeBlocked: tolerance,
						lambda:                   lambda,
						finalProbabilityFunction: finalProbabilityFunction.fn,
					})
				}
			}
		}
	}

	type sweepResult struct {
		variant fairConfigVariant
		result  runResult
	}

	results := make([]sweepResult, 0, len(variants))
	for _, variant := range variants {
		result, err := runScenarioWithVariant(sc, clients, events, entitlements, true, seed, variant)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sweep failed for %s: %v\n", variant.name, err)
			os.Exit(1)
		}
		results = append(results, sweepResult{variant: variant, result: result})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].result.entitlementJain == results[j].result.entitlementJain {
			return results[i].result.resourceUtilization > results[j].result.resourceUtilization
		}
		return results[i].result.entitlementJain > results[j].result.entitlementJain
	})

	fmt.Println("  api_noisy_neighbors FAIR config sweep, ranked by entitlement Jain")
	fmt.Printf("  baseline entitlement_jain=%.4f utilization=%.2f%%\n", baseline.entitlementJain, 100*baseline.resourceUtilization)
	fmt.Println("  rank config                              entitlement_jain improvement utilization successes failures throttled worst_ratio")
	for i := 0; i < min(10, len(results)); i++ {
		result := results[i].result
		improvement := 100 * (result.entitlementJain - baseline.entitlementJain) / baseline.entitlementJain
		fmt.Printf(
			"  %-4d %-35s %.4f           %+6.1f%%     %.2f%%      %-9d %-8d %-9d %.3f\n",
			i+1,
			truncate(results[i].variant.name, 35),
			result.entitlementJain,
			improvement,
			100*result.resourceUtilization,
			result.totalSuccesses,
			result.resourceFailures,
			result.throttled,
			result.worstEntitlementRatio,
		)
	}
	fmt.Println()
}

func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}

func truncate(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength-1] + "."
}

func serviceCapacity(sc scenario, totalRequests int) float64 {
	capacity := sc.capacityPerSecond*sc.duration.Seconds() + sc.burstTokens
	return math.Min(capacity, float64(totalRequests))
}

func maxMinEntitlements(demand []float64, capacity float64) []float64 {
	entitlements := make([]float64, len(demand))
	indexes := make([]int, len(demand))
	for i := range demand {
		indexes[i] = i
	}

	sort.Slice(indexes, func(i, j int) bool {
		return demand[indexes[i]] < demand[indexes[j]]
	})

	for pos, idx := range indexes {
		remainingClients := float64(len(indexes) - pos)
		share := capacity / remainingClients
		if demand[idx] <= share {
			entitlements[idx] = demand[idx]
			capacity -= demand[idx]
			continue
		}

		for _, remainingIndex := range indexes[pos:] {
			entitlements[remainingIndex] = share
		}
		break
	}

	return entitlements
}

func jain(values []float64) float64 {
	var sum float64
	var sumSquares float64
	for _, value := range values {
		sum += value
		sumSquares += value * value
	}

	if sumSquares == 0 {
		return 0
	}
	return sum * sum / (float64(len(values)) * sumSquares)
}

func entitlementJain(successes, entitlements []float64) (float64, float64, float64) {
	ratios := make([]float64, 0, len(successes))
	worst := math.Inf(1)
	var sum float64

	for i := range successes {
		if entitlements[i] == 0 {
			continue
		}
		ratio := successes[i] / entitlements[i]
		ratios = append(ratios, ratio)
		sum += ratio
		worst = math.Min(worst, ratio)
	}

	if len(ratios) == 0 {
		return 0, 0, 0
	}

	return jain(ratios), sum / float64(len(ratios)), worst
}

func printScenario(sc scenario, clientCount int, eventCount int, baseline runResult, withFair runResult) {
	fmt.Printf("Scenario: %s\n", sc.name)
	fmt.Printf("  %s\n", sc.description)
	fmt.Printf("  clients=%d requests=%d duration=%s capacity=%.0f/s\n", clientCount, eventCount, sc.duration, sc.capacityPerSecond)
	fmt.Println("  mode            entitlement_jain raw_jain utilization successes failures throttled mean_ratio worst_ratio")
	printResult(baseline)
	printResult(withFair)
	fmt.Println()
}

func printResult(result runResult) {
	fmt.Printf(
		"  %-15s %.4f           %.4f   %.2f%%      %-9d %-8d %-9d %.3f      %.3f\n",
		result.mode,
		result.entitlementJain,
		result.rawJain,
		100*result.resourceUtilization,
		result.totalSuccesses,
		result.resourceFailures,
		result.throttled,
		result.meanEntitlementRatio,
		result.worstEntitlementRatio,
	)
}
