package gashe_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// downBackend fails every Get and Set, like an unreachable Redis.
type downBackend struct {
	gashe.Backend

	sets int
}

func (*downBackend) Get(context.Context, string) ([]byte, error) { return nil, errBackendDown }

func (d *downBackend) Set(context.Context, string, []byte, time.Duration) error {
	d.sets++
	return errBackendDown
}

// cached binds a cache and options, so a test can call Do like the code under test calls
// gashe.GetOrLoad.
type cached[V any] struct {
	c *gashe.Cache
	o gashe.Policy[V]
}

func newCached[V any](c *gashe.Cache, o gashe.Policy[V]) cached[V] {
	return cached[V]{c: c, o: o}
}

func (s cached[V]) Do(ctx context.Context, key string, load func(context.Context) (V, error)) (V, error) {
	return gashe.GetOrLoad(ctx, s.c, key, s.o, load)
}

func (s cached[V]) Reload(ctx context.Context, key string, load func(context.Context) (V, error)) (V, error) {
	return gashe.Reload(ctx, s.c, key, s.o, load)
}

func counter(v user) (calls *int, load func(context.Context) (user, error)) {
	calls = new(int)
	return calls, func(context.Context) (user, error) {
		*calls++
		return v, nil
	}
}

func TestGetOrLoadAnswersFromCache(t *testing.T) {
	ctx := context.Background()
	s := newCached(newCache(t), gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}})
	calls, load := counter(user{Name: "Ann"})

	for range 3 {
		got, err := s.Do(ctx, "user:1", load)
		if err != nil || got.Name != "Ann" {
			t.Fatalf("Do: %+v %v", got, err)
		}
	}
	if *calls != 1 {
		t.Fatalf("loader ran %d times, want 1", *calls)
	}
}

func TestGetOrLoadDoesNotStoreErrors(t *testing.T) {
	ctx := context.Background()
	s := newCached(newCache(t), gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}})
	errBoom := errors.New("boom")
	calls := 0
	load := func(context.Context) (user, error) {
		calls++
		return user{}, errBoom
	}

	for range 2 {
		if _, err := s.Do(ctx, "user:1", load); !errors.Is(err, errBoom) {
			t.Fatalf("got %v, want the loader error", err)
		}
	}
	if calls != 2 {
		t.Fatalf("loader ran %d times, want 2", calls)
	}
}

func TestGetOrLoadCondition(t *testing.T) {
	ctx := context.Background()
	s := newCached(newCache(t), gashe.Policy[*user]{
		TTL:   gashe.TTL{For: time.Hour},
		Store: gashe.Store[*user]{If: gashe.NotNil[*user]},
	})
	calls := 0
	load := func(context.Context) (*user, error) {
		calls++
		return nil, nil
	}

	_, _ = s.Do(ctx, "user:1", load)
	_, _ = s.Do(ctx, "user:1", load)
	if calls != 2 {
		t.Fatalf("nil result was stored: loader ran %d times, want 2", calls)
	}
}

func TestGetOrLoadPrefix(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	s := newCached(c, gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Prefix: "v2:"})
	_, load := counter(user{Name: "Ann"})

	_, _ = s.Do(ctx, "user:1", load)
	if ok, _ := c.Exists(ctx, "v2:user:1"); !ok {
		t.Fatal("value is not stored under the prefixed key")
	}
}

func TestGetOrLoadReturnsStoredShape(t *testing.T) {
	type withHidden struct {
		Name   string
		hidden int
	}
	ctx := context.Background()
	s := newCached(newCache(t), gashe.Policy[withHidden]{TTL: gashe.TTL{For: time.Hour}})
	load := func(context.Context) (withHidden, error) { return withHidden{Name: "Ann", hidden: 7}, nil }

	miss, _ := s.Do(ctx, "k", load)
	hit, _ := s.Do(ctx, "k", load)
	if miss != hit {
		t.Fatalf("miss %+v and hit %+v differ", miss, hit)
	}
}

func TestGetOrLoadSurvivesBackendFailure(t *testing.T) {
	ctx := context.Background()
	down := &downBackend{Backend: newCache(t).Backend()}
	var reported []error
	onError := func(_ context.Context, err error) { reported = append(reported, err) }
	s := newCached(
		gashe.New(down, gashe.Options{Hooks: gashe.Hooks{OnError: onError}}),
		gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}},
	)

	got, err := s.Do(ctx, "user:1", func(context.Context) (user, error) { return user{Name: "Ann"}, nil })
	if err != nil || got.Name != "Ann" {
		t.Fatalf("Do: %+v %v", got, err)
	}
	if down.sets != 0 {
		t.Fatal("result was stored although the backend failed on read")
	}
	if len(reported) != 1 || !errors.Is(reported[0], errBackendDown) {
		t.Fatalf("OnError got %v, want one backend error", reported)
	}
}

func TestGetOrLoadTTLOfNegativeSkipsStore(t *testing.T) {
	ctx := context.Background()
	s := newCached(newCache(t), gashe.Policy[user]{
		TTL: gashe.TTL{For: func(u user) time.Duration {
			if u.Name == "" {
				return -1
			}
			return time.Hour
		}},
	})
	calls, load := counter(user{})
	_, _ = s.Do(ctx, "k", load)
	_, _ = s.Do(ctx, "k", load)
	if *calls != 2 {
		t.Fatalf("a result with a negative TTL was stored: %d calls", *calls)
	}
}

func TestGetOrLoadRefresh(t *testing.T) {
	ctx := context.Background()
	s := newCached(newCache(t), gashe.Policy[int]{TTL: gashe.TTL{For: time.Hour}})
	_, load := versions()

	_, _ = s.Do(ctx, "k", load)
	if got, err := s.Reload(ctx, "k", load); err != nil || got != 2 {
		t.Fatalf("Refresh = %d, %v; want v2", got, err)
	}
	if got, _ := s.Do(ctx, "k", load); got != 2 {
		t.Fatalf("after Refresh: v%d, want the refreshed v2", got)
	}
	if _, err := s.Reload(ctx, "k", func(context.Context) (int, error) { return 0, errAPI }); !errors.Is(err, errAPI) {
		t.Fatalf("failed Refresh: %v", err)
	}
	if got, _ := s.Do(ctx, "k", load); got != 2 {
		t.Fatalf("a failed Refresh changed the stored v%d", got)
	}
}

func TestGetOrLoadFailoverMode(t *testing.T) {
	ctx := context.Background()
	var reported []error
	c := gashe.New(newCache(t).Backend(), gashe.Options{
		Hooks: gashe.Hooks{OnError: func(_ context.Context, err error) { reported = append(reported, err) }},
	})
	// TTL 0: never fresh, the loader runs on every call; the copy answers its errors.
	s := newCached(c, gashe.Policy[int]{Stale: gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}}})
	calls, load := versions()

	_, _ = s.Do(ctx, "k", load)
	_, _ = s.Do(ctx, "k", load)
	if calls.Load() != 2 {
		t.Fatalf("loader ran %d times, want on every call", calls.Load())
	}
	got, err := s.Do(ctx, "k", func(context.Context) (int, error) { return 0, errAPI })
	if err != nil || got != 2 {
		t.Fatalf("failing call: %d, %v; want the copy v2", got, err)
	}
	if len(reported) != 1 || !errors.Is(reported[0], errAPI) {
		t.Fatalf("OnError got %v", reported)
	}
	errOther := errors.New("bad request")
	only := newCached(c, gashe.Policy[int]{
		Stale: gashe.Stale{
			IfError: gashe.StaleIfError{
				For:       time.Hour,
				IsFailure: func(err error) bool { return errors.Is(err, errAPI) },
			},
		},
	})
	_, _ = only.Do(ctx, "m", load)
	if _, err := only.Do(
		ctx,
		"m",
		func(context.Context) (int, error) { return 0, errOther },
	); !errors.Is(
		err,
		errOther,
	) {
		t.Fatalf("an error Match rejects: %v, want it returned", err)
	}
}

func TestGetOrLoadSharesOnlySameType(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	ints := gashe.Policy[int]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{SingleFlight: true}}
	strs := gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{SingleFlight: true}}
	if _, err := gashe.GetOrLoad(ctx, c, "k", ints, func(context.Context) (int, error) { return 1, nil }); err != nil {
		t.Fatal(err)
	}
	// The same key read as another type is a decode failure: a miss, so the loader runs.
	got, err := gashe.GetOrLoad(ctx, c, "k", strs, func(context.Context) (string, error) { return "one", nil })
	if err != nil || got != "one" {
		t.Fatalf("got %q, %v", got, err)
	}
}

// A shared load outlives the caller that started it: one caller leaving must not fail the others.
func TestSharedLoadOutlivesTheCallerThatStartedIt(t *testing.T) {
	c := newCache(t)
	started, release := make(chan struct{}), make(chan struct{})
	load := func(ctx context.Context) (string, error) {
		close(started)
		select {
		case <-release:
			return "value", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	policy := gashe.Policy[string]{TTL: gashe.TTL{For: time.Minute}, Stampede: gashe.Stampede{SingleFlight: true}}

	first, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := gashe.GetOrLoad(first, c, "k", policy, load)
		firstDone <- err
	}()
	<-started
	second := make(chan string, 1)
	go func() {
		v, err := gashe.GetOrLoad(context.Background(), c, "k", policy, load)
		if err != nil {
			v = "error: " + err.Error()
		}
		second <- v
	}()
	time.Sleep(20 * time.Millisecond) // the second call joins the load
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("the first caller got %v, want its own context.Canceled", err)
	}
	close(release)
	if v := <-second; v != "value" {
		t.Fatalf("the second caller got %q, want the result of the shared load", v)
	}
}

// With LockOnMiss, a process that waited for the lock takes the "not found" the holder stored: one
// load for all processes, also when the answer is "not found".
func TestLockOnMissSharesANegativeAnswer(t *testing.T) {
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	procA := gashe.New(mem, gashe.Options{LockPollInterval: time.Millisecond})
	procB := gashe.New(mem, gashe.Options{LockPollInterval: time.Millisecond})
	errMissing := errors.New("missing")
	p := gashe.Policy[string]{
		TTL:      gashe.TTL{For: time.Minute},
		Stampede: gashe.Stampede{SingleFlight: true, LockFor: 5 * time.Second},
		Negative: gashe.Negative{
			For:        time.Minute,
			IsNotFound: func(err error) bool { return errors.Is(err, errMissing) },
		},
	}
	var loads atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	slow := func(context.Context) (string, error) {
		if loads.Add(1) == 1 {
			close(started)
			<-release
		}
		return "", errMissing
	}
	done := make(chan error, 1)
	go func() { _, err := gashe.GetOrLoad(ctx, procA, "k", p, slow); done <- err }()
	<-started // process A holds the load lock
	waited := make(chan error, 1)
	go func() { _, err := gashe.GetOrLoad(ctx, procB, "k", p, slow); waited <- err }()
	time.Sleep(20 * time.Millisecond) // process B waits for the lock
	close(release)
	if err := <-done; !errors.Is(err, gashe.ErrNotFound) || !errors.Is(err, errMissing) {
		t.Fatalf("process A got %v, want ErrNotFound wrapping the loader error", err)
	}
	if err := <-waited; !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("process B got %v", err)
	}
	if n := loads.Load(); n != 1 {
		t.Fatalf("the loader ran %d times, want 1 for both processes", n)
	}
}

// Reload leaves the stored result alone on any loader error, one Negative.If accepts too.
func TestReloadErrorKeepsTheStoredResult(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	errMissing := errors.New("missing")
	p := gashe.Policy[string]{
		TTL: gashe.TTL{For: time.Hour},
		Negative: gashe.Negative{
			For:        time.Minute,
			IsNotFound: func(err error) bool { return errors.Is(err, errMissing) },
		},
	}
	_, _ = gashe.GetOrLoad(ctx, c, "k", p, func(context.Context) (string, error) { return "kept", nil })
	if _, err := gashe.Reload(
		ctx,
		c,
		"k",
		p,
		func(context.Context) (string, error) { return "", errMissing },
	); !errors.Is(
		err,
		errMissing,
	) {
		t.Fatalf("Reload = %v, want the loader error", err)
	}
	v, err := gashe.GetOrLoad(ctx, c, "k", p, func(context.Context) (string, error) { return "loaded", nil })
	if err != nil || v != "kept" {
		t.Fatalf("after a failed Reload: %q, %v; want the stored result", v, err)
	}
}
