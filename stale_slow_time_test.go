//go:build go1.25

package gashe_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// slowly returns a loader that answers 1, 2, ... after delay and counts its runs.
func slowly(delay time.Duration) (calls *atomic.Int32, load func(context.Context) (int, error)) {
	calls = new(atomic.Int32)
	return calls, func(context.Context) (int, error) {
		n := calls.Add(1)
		if n > 1 {
			time.Sleep(delay) // the first load fills the cache; later ones are slow
		}
		return int(n), nil
	}
}

var slowPolicy = gashe.Policy[int]{
	TTL: gashe.TTL{For: time.Minute},
	Stale: gashe.Stale{
		IfError: gashe.StaleIfError{For: time.Hour},
		IfSlow:  gashe.StaleIfSlow{For: time.Hour, After: 100 * time.Millisecond},
	},
}

func TestStaleIfSlowAnswersWithTheOldResult(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		mem := memory.New(memory.Options{PurgeInterval: -1})
		t.Cleanup(func() { _ = mem.Close() })
		var results []gashe.Result
		c := gashe.New(mem, gashe.Options{
			Hooks: gashe.Hooks{OnResult: func(_ context.Context, o gashe.Outcome) {
				results = append(results, o.Result)
			}},
		})
		calls, load := slowly(5 * time.Second)

		_, _ = gashe.GetOrLoad(ctx, c, "k", slowPolicy, load)
		time.Sleep(2 * time.Minute) // stale, within StaleIfError
		start := time.Now()
		if got, err := gashe.GetOrLoad(ctx, c, "k", slowPolicy, load); err != nil || got != 1 {
			t.Fatalf("slow source: v%d, %v; want the old v1", got, err)
		}
		if waited := time.Since(start); waited != 100*time.Millisecond {
			t.Fatalf("waited %v, want StaleIfSlow", waited)
		}
		if got, _ := gashe.GetOrLoad(ctx, c, "k", slowPolicy, load); got != 1 || calls.Load() != 2 {
			t.Fatalf("while the load runs: v%d after %d loads; want v1 at once and no new load", got, calls.Load())
		}
		time.Sleep(5 * time.Second) // the background load finishes and stores v2
		synctest.Wait()
		if got, _ := gashe.GetOrLoad(ctx, c, "k", slowPolicy, load); got != 2 || calls.Load() != 2 {
			t.Fatalf("after the load: v%d after %d loads; want the stored v2", got, calls.Load())
		}
		want := []gashe.Result{gashe.ResultMiss, gashe.ResultStaleIfSlow, gashe.ResultStaleIfSlow, gashe.ResultHit}
		if len(results) != len(want) {
			t.Fatalf("results %v, want %v", results, want)
		}
		for i := range want {
			if results[i] != want[i] {
				t.Fatalf("results %v, want %v", results, want)
			}
		}
	})
}

func TestStaleIfSlowWaitsForAFastLoader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		_, load := slowly(10 * time.Millisecond)

		_, _ = gashe.GetOrLoad(ctx, c, "k", slowPolicy, load)
		time.Sleep(2 * time.Minute)
		if got, err := gashe.GetOrLoad(ctx, c, "k", slowPolicy, load); err != nil || got != 2 {
			t.Fatalf("fast source: v%d, %v; want the new v2", got, err)
		}
	})
}

func TestStaleIfSlowReportsALateFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		mem := memory.New(memory.Options{PurgeInterval: -1})
		t.Cleanup(func() { _ = mem.Close() })
		var reported []error
		c := gashe.New(
			mem,
			gashe.Options{
				Hooks: gashe.Hooks{OnError: func(_ context.Context, err error) { reported = append(reported, err) }},
			},
		)
		var calls atomic.Int32
		load := func(context.Context) (int, error) {
			if calls.Add(1) == 1 {
				return 1, nil
			}
			time.Sleep(time.Second)
			return 0, errAPI
		}

		_, _ = gashe.GetOrLoad(ctx, c, "k", slowPolicy, load)
		time.Sleep(2 * time.Minute)
		_, _ = gashe.GetOrLoad(ctx, c, "k", slowPolicy, load)
		time.Sleep(time.Second)
		synctest.Wait()
		if len(reported) != 1 || !errors.Is(reported[0], errAPI) {
			t.Fatalf("reported %v; the failure of the background load must reach OnError", reported)
		}
	})
}

// Stale.IfSlow has a window of its own: it answers a slow loader without IfError, and only within
// its For.
func TestStaleIfSlowHasItsOwnWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		p := gashe.Policy[int]{
			TTL:   gashe.TTL{For: time.Minute},
			Stale: gashe.Stale{IfSlow: gashe.StaleIfSlow{For: 10 * time.Minute, After: 100 * time.Millisecond}},
		}
		_, load := slowly(5 * time.Second)
		_, _ = gashe.GetOrLoad(ctx, c, "k", p, load)

		time.Sleep(5 * time.Minute) // within IfSlow.For
		start := time.Now()
		if got, _ := gashe.GetOrLoad(ctx, c, "k", p, load); got != 1 || time.Since(start) > time.Second {
			t.Fatalf("within IfSlow.For: got %d after %v, want the stale 1 at once", got, time.Since(start))
		}
		time.Sleep(time.Minute) // the background load stored 2 meanwhile

		time.Sleep(20 * time.Minute) // past TTL + IfSlow.For: nothing stale is left
		start = time.Now()
		if got, _ := gashe.GetOrLoad(ctx, c, "k", p, load); got != 3 || time.Since(start) < 5*time.Second {
			t.Fatalf("past IfSlow.For: got %d after %v, want the call to wait for 3", got, time.Since(start))
		}
	})
}
