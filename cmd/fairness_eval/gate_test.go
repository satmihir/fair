package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

// Budgets are per seed: neither a lucky trace nor a good average may mask starvation.
func TestFairnessGate(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		classes                    []workloadClass
		minWorst, minJain, minUtil float64
	}{
		{"equal_clients", []workloadClass{{name: "peer", clients: 20, ratePerSecond: 4}}, .70, .98, .98},
		{"noisy_neighbors", []workloadClass{{name: "steady", clients: 16, ratePerSecond: 2}, {name: "noisy", clients: 4, ratePerSecond: 20}}, .50, .75, .98},
	} {
		sc := scenario{name: tc.name, classes: tc.classes, duration: 120 * time.Second, rotationPeriod: 25 * time.Second, capacityPerSecond: 40, burstTokens: 40}
		for _, seed := range []int64{17, 53, 101} {
			t.Run(fmt.Sprintf("%s/seed_%d", tc.name, seed), func(t *testing.T) {
				run := func() runResult {
					clients, events := generateEvents(sc, seed)
					entitlements := maxMinEntitlements(requestCounts(len(clients), events), serviceCapacity(sc, len(events)))
					r, err := runScenario(sc, clients, events, entitlements, true, seed)
					if err != nil {
						t.Fatal(err)
					}
					return r
				}
				r := run()
				if !reflect.DeepEqual(r, run()) {
					t.Fatal("identical seed produced different results")
				}
				if r.rotations != 4 {
					t.Fatalf("got %d rotations; want 4", r.rotations)
				}
				for _, budget := range []struct {
					name       string
					got, floor float64
				}{
					{"worst entitlement", r.worstEntitlementRatio, tc.minWorst},
					{"entitlement Jain", r.entitlementJain, tc.minJain},
					{"utilization", r.resourceUtilization, tc.minUtil},
				} {
					t.Logf("%s = %.4f (floor %.2f)", budget.name, budget.got, budget.floor)
					if !(budget.got >= budget.floor) {
						t.Errorf("%s %.4f below %.2f", budget.name, budget.got, budget.floor)
					}
				}
			})
		}
	}
}
