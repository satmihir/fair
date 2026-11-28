package tracker_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/satmihir/fair/pkg/config"
	"github.com/satmihir/fair/pkg/request"
	"github.com/satmihir/fair/pkg/tracker"
)

// BenchmarkProduction measures a fixed amount of API work per iteration,
// independent of the probabilistic throttle decision. See designs/production-performance.md.
func BenchmarkProduction(b *testing.B) {
	for _, keys := range []int{1, 4096} {
		for _, stats := range []bool{false, true} {
			b.Run(fmt.Sprintf("keys=%d/stats=%t", keys, stats), func(b *testing.B) {
				benchmarkProduction(b, keys, stats)
			})
		}
	}
}

func benchmarkProduction(b *testing.B, keyCount int, stats bool) {
	b.Helper()
	cfg := config.DefaultFairnessTrackerConfig()
	cfg.L, cfg.M = 3, 2048
	cfg.RotationFrequency, cfg.IncludeStats = 25*time.Millisecond, stats
	ft, err := tracker.NewFairnessTracker(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer ft.Close()
	keys := make([][8]byte, keyCount)
	for i := range keys {
		binary.LittleEndian.PutUint64(keys[i][:], uint64(i))
	}

	// Bound the sample storage even for long runs; no shared counter per call.
	stride := max(64, b.N/65536+1)
	var worker atomic.Uint64
	var mu sync.Mutex
	var samples []int64
	stop := make(chan struct{})
	peak := make(chan uint64, 1)
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		var high uint64
		for {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			high = max(high, m.HeapAlloc)
			select {
			case <-stop:
				peak <- high
				return
			case <-tick.C:
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	before := ft.RotationStatus()
	b.RunParallel(func(pb *testing.PB) {
		id := int(worker.Add(1) - 1)
		var local []int64
		for i := 0; pb.Next(); i++ {
			key := keys[(id*997+i)%keyCount][:]
			outcome := request.OutcomeSuccess
			if (id+i)%8 == 0 {
				outcome = request.OutcomeFailure
			}
			sample := i%stride == 0
			var start time.Time
			if sample {
				start = time.Now()
			}
			ft.RegisterRequest(context.Background(), key)
			ft.ReportOutcome(context.Background(), key, outcome)
			if sample {
				local = append(local, time.Since(start).Nanoseconds())
			}
		}
		mu.Lock()
		samples = append(samples, local...)
		mu.Unlock()
	})
	after := ft.RotationStatus()
	b.StopTimer()
	b.ReportMetric(float64(after.Rotations-before.Rotations), "promotions/run")
	close(stop)
	b.ReportMetric(float64(<-peak)/(1<<20), "sampled-heap-MiB")
	if after.Failures != 0 {
		b.Fatalf("rotation failed: %+v", after)
	}
	slices.Sort(samples)
	b.ReportMetric(float64(samples[len(samples)-(len(samples)/100)-1]), "p99-ns/pair")
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "pairs/s")
}
