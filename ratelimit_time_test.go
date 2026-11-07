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
		limit := gashe.FixedWindow{Calls: 1, Per: time.Minute, BanFor: 10 * time.Minute}

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

// retryAfter calls RateLimit, which must refuse, and returns its RetryAfter.
func retryAfter(t *testing.T, c *gashe.Cache, l gashe.Limit) time.Duration {
	t.Helper()
	err := c.RateLimit(context.Background(), "k", l)
	var limited *gashe.RateLimitError
	if !errors.As(err, &limited) || !errors.Is(err, gashe.ErrRateLimited) {
		t.Fatalf("err = %v, want a *RateLimitError that is ErrRateLimited", err)
	}
	return limited.RetryAfter
}

func TestRateLimitRetryAfterFixed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		limit := gashe.FixedWindow{Calls: 1, Per: time.Minute, BanFor: 10 * time.Minute}

		_ = c.RateLimit(ctx, "k", limit)
		if got := retryAfter(t, c, limit); got != 10*time.Minute {
			t.Fatalf("first refusal: RetryAfter = %v, want the ban of 10m", got)
		}
		time.Sleep(9 * time.Minute)
		if got := retryAfter(t, c, limit); got != time.Minute {
			t.Fatalf("during the ban: RetryAfter = %v, want what is left of it, 1m", got)
		}
	})
}

func TestRateLimitRetryAfterSliding(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		limit := gashe.PerMinute(2)
		begin := time.Now() // on a minute boundary, as in TestRateLimitSliding

		_ = c.RateLimit(ctx, "k", limit)
		time.Sleep(10 * time.Second)
		_ = c.RateLimit(ctx, "k", limit)
		time.Sleep(10 * time.Second)
		// At 20s the window is full: wait until it ends at 60s and then weighs 1 call, at 90s.
		if got := retryAfter(t, c, limit); got != 70*time.Second {
			t.Fatalf("at 20s: RetryAfter = %v, want 70s", got)
		}
		time.Sleep(time.Until(begin.Add(65 * time.Second)))
		// At 65s the last window weighs 2 * 55/60; it weighs 1 at 90s.
		got := retryAfter(t, c, limit)
		if got != 25*time.Second {
			t.Fatalf("at 65s: RetryAfter = %v, want 25s", got)
		}
		time.Sleep(got)
		if err := c.RateLimit(ctx, "k", limit); err != nil {
			t.Fatalf("after RetryAfter: %v", err)
		}
	})
}
