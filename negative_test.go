package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

var errNoRows = errors.New("no rows")

func TestGetOrLoadNegative(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.Policy[user]{
		TTL: time.Hour,
		Negative: gashe.Negative{
			If:  func(err error) bool { return errors.Is(err, errNoRows) },
			Err: errNoRows,
			TTL: time.Minute,
		},
	}
	calls := 0
	missing := func(context.Context) (user, error) {
		calls++
		return user{}, errNoRows
	}

	for range 3 {
		if _, err := gashe.GetOrLoad(ctx, c, "user:999", users, missing); !errors.Is(err, errNoRows) {
			t.Fatalf("got %v, want errNoRows", err)
		}
	}
	if calls != 1 {
		t.Fatalf("loader ran %d times for a missing user, want 1", calls)
	}
	if _, err := gashe.GetOrLoad(ctx, c, "user:1", users, func(context.Context) (user, error) {
		return user{}, errAPI
	}); !errors.Is(err, errAPI) {
		t.Fatalf("an error Match rejects: %v, want it returned", err)
	}
	if ok, _ := c.Exists(ctx, "user:1"); ok {
		t.Fatal("an error Match rejects was stored")
	}
}

func TestGetOrLoadTags(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	tagged := gashe.Policy[int]{TTL: time.Hour, Tags: []string{"users"}}
	other := gashe.Policy[int]{TTL: time.Hour, Tags: []string{"orders"}}
	calls, load := versions()

	_, _ = gashe.GetOrLoad(ctx, c, "a", tagged, load)
	_, _ = gashe.GetOrLoad(ctx, c, "b", other, load)
	if err := c.InvalidateTags(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "a", tagged, load); got != 3 {
		t.Fatalf("after InvalidateTags: v%d, want a reload", got)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "b", other, load); got != 2 {
		t.Fatalf("a key of another tag: v%d, want the stored v2", got)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "a", tagged, load); got != 3 || calls.Load() != 3 {
		t.Fatalf("after the reload: v%d, %d calls; want the stored v3", got, calls.Load())
	}
	if got, _ := gashe.Reload(ctx, c, "a", tagged, load); got != 4 {
		t.Fatalf("Reload: v%d", got)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "a", tagged, load); got != 4 {
		t.Fatalf("after Reload: v%d, want the reloaded v4 kept with the current generation", got)
	}
}

func TestGetOrLoadOnResult(t *testing.T) {
	ctx := context.Background()
	var got []gashe.Result
	c := gashe.New(newCache(t).Backend(), gashe.Options{
		OnResult: func(_ context.Context, _ string, r gashe.Result) { got = append(got, r) },
	})
	o := gashe.Policy[int]{TTL: time.Hour, StaleIfError: time.Hour}
	_, load := versions()

	_, _ = gashe.GetOrLoad(ctx, c, "k", o, load) // miss
	_, _ = gashe.GetOrLoad(ctx, c, "k", o, load) // hit
	if len(got) != 2 || got[0] != gashe.ResultMiss || got[1] != gashe.ResultHit {
		t.Fatalf("OnResult got %v, want [miss hit]", got)
	}
	if gashe.ResultStaleIfError.String() != "stale-if-error" {
		t.Fatalf("String = %q", gashe.ResultStaleIfError.String())
	}
}

func TestGetOrLoadJitter(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.Policy[int]{TTL: time.Hour, Jitter: 0.5}
	seen := map[time.Duration]bool{}
	for i := range 20 {
		key := "k" + string(rune('a'+i))
		_, _ = gashe.GetOrLoad(ctx, c, key, o, func(context.Context) (int, error) { return i, nil })
		ttl, _ := c.TTL(ctx, key)
		if ttl < 30*time.Minute || ttl > 90*time.Minute {
			t.Fatalf("TTL %v outside 1h +-50%%", ttl)
		}
		seen[ttl.Round(time.Second)] = true
	}
	if len(seen) < 2 {
		t.Fatal("Jitter did not spread the TTLs")
	}
}

func TestCircuitBreakerUsesNoBackend(t *testing.T) {
	c := gashe.New(incrDown{Backend: lockDown{Backend: newCache(t).Backend()}}, gashe.Options{})
	o := gashe.BreakerOptions{FailurePercent: 50, Window: time.Minute, OpenFor: time.Minute}
	_ = through(c, o, errAPI)
	if err := through(c, o, nil); !errors.Is(err, gashe.ErrCircuitOpen) {
		t.Fatalf("got %v, want the breaker open without any backend", err)
	}
}
