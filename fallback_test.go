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

// withFallback returns a cache over a dead backend, a fallback for its policies, and what
// OnResult saw.
func withFallback(t *testing.T, maxTTL time.Duration) (*gashe.Cache, gashe.Fallback, *[]gashe.Result) {
	t.Helper()
	spare := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = spare.Close() })
	results := new([]gashe.Result)
	c := gashe.New(deadBackend{Backend: newCache(t).Backend()}, gashe.Options{
		Hooks: gashe.Hooks{
			OnResult: func(_ context.Context, o gashe.Outcome) { *results = append(*results, o.Result) },
		},
	})
	return c, gashe.Fallback{Backend: spare, MaxTTL: maxTTL}, results
}

func TestFallbackAnswersWhileTheBackendIsDown(t *testing.T) {
	ctx := context.Background()
	c, fb, results := withFallback(t, 0)
	calls, load := counter(user{Name: "Ann"})
	users := gashe.For(c, gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Fallback: fb})

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
		_, _ = gashe.GetOrLoad(ctx, c, "42", gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}}, load)
	}
	if *calls != 3 {
		t.Fatalf("loader ran %d times, want 3 without a fallback", *calls)
	}
}

func TestFallbackKeepsListsAndReloads(t *testing.T) {
	ctx := context.Background()
	c, fb, _ := withFallback(t, 0)
	users := gashe.For(c, gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Prefix: "u:", Fallback: fb})
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
	c, _, _ := withFallback(t, 0)
	if _, ok, err := c.TryLock(context.Background(), "job", time.Minute); ok || !errors.Is(err, errBackendDown) {
		t.Fatalf("TryLock = %v, %v; a lock must not be taken in the memory of one process", ok, err)
	}
}

func TestInvalidFallbackIsRejected(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	_, load := counter(user{})
	for name, fb := range map[string]gashe.Fallback{
		"MaxTTL without a Backend": {MaxTTL: time.Second},
		"negative MaxTTL":          {Backend: memory.New(memory.Options{PurgeInterval: -1}), MaxTTL: -time.Second},
	} {
		_, err := gashe.GetOrLoad(ctx, c, "k", gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Fallback: fb}, load)
		if !errors.Is(err, gashe.ErrInvalidOptions) {
			t.Errorf("%s: %v, want ErrInvalidOptions", name, err)
		}
	}
}

func TestFallbackIsPerPolicy(t *testing.T) {
	ctx := context.Background()
	c, fb, _ := withFallback(t, 0)
	kept, keptLoad := counter(user{Name: "Ann"})
	plain, plainLoad := counter(user{Name: "Ann"})
	with := gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Prefix: "kept:", Fallback: fb}
	without := gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Prefix: "plain:"}
	for range 2 {
		_, _ = gashe.GetOrLoad(ctx, c, "42", with, keptLoad)
		_, _ = gashe.GetOrLoad(ctx, c, "42", without, plainLoad)
	}
	if *kept != 1 || *plain != 2 {
		t.Fatalf("loads: %d with Fallback, %d without; want 1 and 2", *kept, *plain)
	}
}

func TestFallbackKeepsNotFound(t *testing.T) {
	ctx := context.Background()
	c, fb, results := withFallback(t, 0)
	calls := 0
	missing := func(context.Context) (user, error) { calls++; return user{}, errAPI }
	users := gashe.For(c, gashe.Policy[user]{
		TTL: gashe.TTL{For: time.Hour},
		Negative: gashe.Negative{
			For:        time.Minute,
			IsNotFound: func(err error) bool { return errors.Is(err, errAPI) },
		},
		Fallback: fb,
	})
	for range 3 {
		if _, err := users.GetOrLoad(ctx, "ghost", missing); !errors.Is(err, gashe.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	}
	if calls != 1 {
		t.Fatalf("loader ran %d times, want once: the fallback keeps the not found too", calls)
	}
	if (*results)[2] != gashe.ResultNegativeHit {
		t.Fatalf("results %v, want a negative hit from the fallback", *results)
	}
}
