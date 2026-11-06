package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// deadBackend fails every call, like a Redis that is gone.
type deadBackend struct {
	gashe.Backend
}

func (deadBackend) Get(context.Context, string) ([]byte, error) { return nil, errBackendDown }

func (deadBackend) GetMany(context.Context, ...string) ([][]byte, error) { return nil, errBackendDown }

func (deadBackend) Set(context.Context, string, []byte, time.Duration) error { return errBackendDown }

func (deadBackend) SetIfAbsent(context.Context, string, []byte, time.Duration) (bool, error) {
	return false, errBackendDown
}

// withFallback returns a cache over a dead backend with a fallback, and what OnResult saw.
func withFallback(t *testing.T, maxTTL time.Duration) (*gashe.Cache, *[]gashe.Result) {
	t.Helper()
	spare := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = spare.Close() })
	results := new([]gashe.Result)
	c := gashe.New(deadBackend{Backend: newCache(t).Backend()}, gashe.Options{
		Fallback: &gashe.FallbackOptions{Backend: spare, MaxTTL: maxTTL},
		OnResult: func(_ context.Context, o gashe.Outcome) { *results = append(*results, o.Result) },
	})
	return c, results
}

func TestFallbackAnswersWhileTheBackendIsDown(t *testing.T) {
	ctx := context.Background()
	c, results := withFallback(t, 0)
	calls, load := counter(user{Name: "Ann"})
	users := gashe.For(c, gashe.Policy[user]{TTL: time.Hour})

	for range 3 {
		if got, err := users.GetOrLoad(ctx, "42", load); err != nil || got.Name != "Ann" {
			t.Fatalf("got %+v, %v", got, err)
		}
	}
	if *calls != 1 {
		t.Fatalf("loader ran %d times, want once: the fallback must answer the rest", *calls)
	}
	want := []gashe.Result{gashe.ResultMiss, gashe.ResultFallback, gashe.ResultFallback}
	if len(*results) != 3 || (*results)[1] != want[1] || (*results)[2] != want[2] {
		t.Fatalf("results %v, want %v", *results, want)
	}
}

func TestWithoutFallbackEveryCallLoads(t *testing.T) {
	ctx := context.Background()
	c := gashe.New(deadBackend{Backend: newCache(t).Backend()}, gashe.Options{})
	calls, load := counter(user{Name: "Ann"})
	for range 3 {
		_, _ = gashe.GetOrLoad(ctx, c, "42", gashe.Policy[user]{TTL: time.Hour}, load)
	}
	if *calls != 3 {
		t.Fatalf("loader ran %d times, want 3 without a fallback", *calls)
	}
}

func TestFallbackKeepsListsAndReloads(t *testing.T) {
	ctx := context.Background()
	c, _ := withFallback(t, 0)
	users := gashe.For(c, gashe.Policy[user]{TTL: time.Hour, Prefix: "u:"})
	asked := 0
	loadMany := func(_ context.Context, missing []string) (map[string]user, error) {
		asked += len(missing)
		out := map[string]user{}
		for _, k := range missing {
			out[k] = user{Name: k}
		}
		return out, nil
	}
	_, _ = users.GetOrLoadMany(ctx, []string{"1", "2"}, loadMany)
	got, err := users.GetOrLoadMany(ctx, []string{"1", "2", "3"}, loadMany)
	if err != nil || len(got) != 3 || asked != 3 {
		t.Fatalf("got %v, %v after loading %d keys; want 1 and 2 from the fallback", got, err, asked)
	}

	_, _ = users.Reload(ctx, "9", func(context.Context) (user, error) { return user{Name: "reloaded"}, nil })
	if v, _ := users.GetOrLoad(
		ctx,
		"9",
		func(context.Context) (user, error) { return user{}, nil },
	); v.Name != "reloaded" {
		t.Fatalf("GetOrLoad after Reload = %+v, want the reloaded result from the fallback", v)
	}
}

func TestFallbackLeavesLocksAlone(t *testing.T) {
	c, _ := withFallback(t, 0)
	if _, ok, err := c.TryLock(context.Background(), "job", time.Minute); ok || !errors.Is(err, errBackendDown) {
		t.Fatalf("TryLock = %v, %v; a lock must not be taken in the memory of one process", ok, err)
	}
}

func TestNewPanicsOnInvalidFallback(t *testing.T) {
	for name, o := range map[string]*gashe.FallbackOptions{
		"no backend":      {},
		"negative MaxTTL": {Backend: memory.New(memory.Options{PurgeInterval: -1}), MaxTTL: -time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("no panic")
				}
			}()
			gashe.New(newCache(t).Backend(), gashe.Options{Fallback: o})
		})
	}
}
