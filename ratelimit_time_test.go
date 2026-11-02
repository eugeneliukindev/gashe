//go:build go1.25

package gashe_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestRateLimitFixedBan(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		limit := gashe.Fixed{Calls: 1, Per: time.Minute, BanFor: 10 * time.Minute}

		_ = c.RateLimit(ctx, "k", limit)
		time.Sleep(30 * time.Second)
		_ = c.RateLimit(ctx, "k", limit) // over the limit: banned for 10 minutes from now
		time.Sleep(9 * time.Minute)
		if err := c.RateLimit(ctx, "k", limit); !errors.Is(err, gashe.ErrRateLimited) {
			t.Fatalf("during the ban: %v, want ErrRateLimited", err)
		}
		time.Sleep(time.Minute)
		if err := c.RateLimit(ctx, "k", limit); err != nil {
			t.Fatalf("after the ban: %v", err)
		}
	})
}

func TestRateLimitSliding(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		limit := gashe.PerMinute(2)
		begin := time.Now()

		// The bubble clock starts on a minute boundary, so windows are [0s, 60s), [60s, 120s), ...
		for _, step := range []struct {
			at      time.Duration
			limited bool
		}{
			{0, false},
			{10 * time.Second, false},
			{20 * time.Second, true},   // a third call within a minute
			{65 * time.Second, true},   // 2 * 55/60 of the last window + 1 > 2
			{110 * time.Second, false}, // 2 * 10/60 + 1 <= 2; refused calls were not counted
			{125 * time.Second, false}, // 1 * 55/60 + 1 <= 2
		} {
			time.Sleep(time.Until(begin.Add(step.at)))
			err := c.RateLimit(ctx, "k", limit)
			if got := errors.Is(err, gashe.ErrRateLimited); got != step.limited {
				t.Fatalf("at %v: limited = %v, want %v", step.at, got, step.limited)
			}
		}
	})
}
