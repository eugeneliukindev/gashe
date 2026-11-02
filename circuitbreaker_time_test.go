//go:build go1.25

package gashe_test

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestCircuitBreakerOpensAndCloses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newCache(t)
		o := gashe.BreakerOptions{FailurePercent: 50, MinCalls: 3, Window: time.Minute, OpenFor: 5 * time.Minute}

		_ = through(c, o, nil)
		_ = through(c, o, errAPI) // 1 of 2 failed, but fewer than MinCalls
		if err := through(c, o, nil); err != nil {
			t.Fatalf("below MinCalls: %v", err)
		}
		_ = through(c, o, errAPI) // 2 of 4 failed: 50%, opens

		if _, err := c.CircuitBreaker("api", o); !errors.Is(err, gashe.ErrCircuitOpen) {
			t.Fatalf("open breaker: %v, want ErrCircuitOpen", err)
		}
		time.Sleep(5 * time.Minute)
		if err := through(c, o, nil); err != nil {
			t.Fatalf("after OpenFor: %v", err)
		}
		// Opening cleared the counters: one failure among fresh calls is below MinCalls.
		_ = through(c, o, errAPI)
		if err := through(c, o, nil); err != nil {
			t.Fatalf("counters were not cleared on opening: %v", err)
		}
	})
}

func TestCircuitBreakerTrials(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newCache(t)
		o := gashe.BreakerOptions{FailurePercent: 50, Window: time.Minute, OpenFor: 5 * time.Minute, HalfOpenCalls: 1}
		_ = through(c, o, errAPI)   // opens
		time.Sleep(5 * time.Minute) // one trial

		if err := through(c, o, errAPI); !errors.Is(err, errAPI) {
			t.Fatalf("trial: %v, want it let through", err)
		}
		if err := through(c, o, nil); !errors.Is(err, gashe.ErrCircuitOpen) {
			t.Fatalf("after a failed trial: %v, want open again", err)
		}

		time.Sleep(5 * time.Minute)
		if err := through(c, o, nil); err != nil {
			t.Fatalf("successful trial: %v", err)
		}
		for range 5 {
			if err := through(c, o, nil); err != nil {
				t.Fatalf("after a successful trial: %v, want closed", err)
			}
		}
	})
}

func TestCircuitBreakerTrialSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newCache(t)
		o := gashe.BreakerOptions{FailurePercent: 50, Window: time.Minute, OpenFor: time.Minute, HalfOpenCalls: 1}
		_ = through(c, o, errAPI)
		time.Sleep(time.Minute)

		done, err := c.CircuitBreaker("api", o) // the only trial, still running
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.CircuitBreaker("api", o); !errors.Is(err, gashe.ErrCircuitOpen) {
			t.Fatalf("second call during the trial: %v, want ErrCircuitOpen", err)
		}
		done(nil)
	})
}
