package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backendtest"
)

// flaky fails every Get with errBackendDown and counts the calls that reached it.
type flaky struct {
	gashe.Backend

	gets  int
	pings int
}

func (f *flaky) Get(context.Context, string) ([]byte, error) {
	f.gets++
	return nil, errBackendDown
}

func (f *flaky) Ping(context.Context) error {
	f.pings++
	return errBackendDown
}

var cacheBreaker = gashe.BreakerOptions{FailurePercent: 50, MinCalls: 3, Window: time.Minute, OpenFor: time.Minute}

func newFlaky(t *testing.T) *flaky {
	t.Helper()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	return &flaky{Backend: mem}
}

func TestWithBreakerStopsCallingAFailingBackend(t *testing.T) {
	ctx := context.Background()
	f := newFlaky(t)
	b := gashe.WithBreaker(f, cacheBreaker)

	for range 3 {
		if _, err := b.Get(ctx, "k"); !errors.Is(err, errBackendDown) {
			t.Fatalf("err = %v, want the backend error", err)
		}
	}
	if _, err := b.Get(ctx, "k"); !errors.Is(err, gashe.ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen once the breaker opened", err)
	}
	if f.gets != 3 {
		t.Fatalf("backend got %d calls, want 3: an open breaker must not call it", f.gets)
	}
	if err := b.Ping(ctx); !errors.Is(err, errBackendDown) || f.pings != 1 {
		t.Fatalf("Ping = %v after %d pings; it must reach the backend", err, f.pings)
	}
}

func TestWithBreakerIgnoresMisses(t *testing.T) {
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	b := gashe.WithBreaker(mem, cacheBreaker)

	for range 10 {
		if _, err := b.Get(ctx, "missing"); !errors.Is(err, gashe.ErrNotFound) {
			t.Fatalf("err = %v; a miss is an answer, not a failure", err)
		}
	}
}

func TestGetOrLoadThroughAnOpenBreaker(t *testing.T) {
	ctx := context.Background()
	c := gashe.New(gashe.WithBreaker(newFlaky(t), cacheBreaker), gashe.Options{})
	_, load := versions()
	o := gashe.Policy[int]{TTL: time.Minute}

	for i := range 5 {
		if got, err := gashe.GetOrLoad(ctx, c, "k", o, load); err != nil || got != i+1 {
			t.Fatalf("call %d: %d, %v; the loader must answer while the cache is down", i, got, err)
		}
	}
}

func TestWithBreakerPanicsOnInvalidOptions(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	gashe.WithBreaker(newFlaky(t), gashe.BreakerOptions{})
}

func TestWithBreakerContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		t.Helper()
		mem := memory.New(memory.Options{PurgeInterval: -1})
		t.Cleanup(func() { _ = mem.Close() })
		return gashe.WithBreaker(mem, cacheBreaker)
	})
}
