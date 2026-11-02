package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

// incrDown fails every Incr, like an unreachable Redis.
type incrDown struct {
	gashe.Backend
}

func (incrDown) Incr(context.Context, string, int64, time.Duration) (int64, error) {
	return 0, errBackendDown
}

func TestRateLimitFixed(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	limit := gashe.FixedWindow{Calls: 2, Per: time.Minute}
	for i := range 2 {
		if err := c.RateLimit(ctx, "user:1", limit); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if err := c.RateLimit(ctx, "user:1", limit); !errors.Is(err, gashe.ErrRateLimited) {
		t.Fatalf("third call: %v, want ErrRateLimited", err)
	}
	if err := c.RateLimit(ctx, "user:2", limit); err != nil {
		t.Fatalf("another key is limited too: %v", err)
	}
}

func TestRateLimitInvalid(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	for _, l := range []gashe.Limit{gashe.SlidingWindow{}, gashe.FixedWindow{Calls: 1}, gashe.PerSecond(0)} {
		if err := c.RateLimit(ctx, "k", l); !errors.Is(err, gashe.ErrInvalidOptions) {
			t.Errorf("%+v: %v, want ErrInvalidOptions", l, err)
		}
	}
}

func TestRateLimitLetsCallsThroughWhenBackendFails(t *testing.T) {
	ctx := context.Background()
	var reported int
	c := gashe.New(incrDown{Backend: newCache(t).Backend()}, gashe.Options{
		OnError: func(context.Context, error) { reported++ },
	})
	for range 3 {
		if err := c.RateLimit(ctx, "k", gashe.FixedWindow{Calls: 1, Per: time.Minute}); err != nil {
			t.Fatalf("fixed: %v", err)
		}
		if err := c.RateLimit(ctx, "k", gashe.PerMinute(1)); err != nil {
			t.Fatalf("sliding: %v", err)
		}
	}
	if reported != 6 {
		t.Fatalf("OnError called %d times, want 6", reported)
	}
}
