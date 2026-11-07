package gashe_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestGetOrLoadReloadsAfterTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newCached(newCache(t), gashe.Policy[user]{TTL: gashe.TTL{For: time.Minute}})
		calls, load := counter(user{Name: "Ann"})

		_, _ = s.Do(ctx, "user:1", load)
		time.Sleep(59 * time.Second)
		_, _ = s.Do(ctx, "user:1", load)
		if *calls != 1 {
			t.Fatalf("reloaded before TTL: %d", *calls)
		}
		time.Sleep(time.Second)
		_, _ = s.Do(ctx, "user:1", load)
		if *calls != 2 {
			t.Fatalf("not reloaded after TTL: %d", *calls)
		}
	})
}

func TestGetOrLoadTTLOf(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		s := newCached(c, gashe.Policy[user]{
			TTL: gashe.TTL{For: func(u user) time.Duration {
				if u.Name == "admin" {
					return time.Minute
				}
				return time.Hour
			}},
		})
		_, load := counter(user{Name: "admin"})

		_, _ = s.Do(ctx, "user:1", load)
		if ttl, _ := c.TTL(ctx, "user:1"); ttl != time.Minute {
			t.Fatalf("TTL = %v, want 1m", ttl)
		}
	})
}

func TestGetOrLoadSharesConcurrentLoads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newCached(
			newCache(t),
			gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{SingleFlight: true}},
		)
		var calls atomic.Int32
		load := func(context.Context) (user, error) {
			calls.Add(1)
			time.Sleep(time.Second)
			return user{Name: "Ann"}, nil
		}

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				if got, err := s.Do(ctx, "user:1", load); err != nil || got.Name != "Ann" {
					t.Errorf("Do: %+v %v", got, err)
				}
			})
		}
		wg.Wait()
		if n := calls.Load(); n != 1 {
			t.Fatalf("loader ran %d times for concurrent calls, want 1", n)
		}
	})
}

func TestWithoutSingleFlightEveryCallLoads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newCached(newCache(t), gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}})
		var calls atomic.Int32
		load := func(context.Context) (user, error) {
			calls.Add(1)
			time.Sleep(time.Second)
			return user{Name: "Ann"}, nil
		}

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				if got, err := s.Do(ctx, "user:1", load); err != nil || got.Name != "Ann" {
					t.Errorf("Do: %+v %v", got, err)
				}
			})
		}
		wg.Wait()
		if n := calls.Load(); n != 10 {
			t.Fatalf("loader ran %d times for 10 concurrent calls, want 10", n)
		}
	})
}

func TestGetOrLoadWaiterStopsOnItsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newCached(
			newCache(t),
			gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{SingleFlight: true}},
		)
		release := make(chan struct{})
		var calls atomic.Int32
		load := func(ctx context.Context) (user, error) {
			calls.Add(1)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return user{}, ctx.Err()
		}

		go func() { _, _ = s.Do(context.Background(), "k", load) }()
		synctest.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := s.Do(ctx, "k", load); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiter got %v, want its own deadline", err)
		}
		if n := calls.Load(); n != 1 {
			t.Fatalf("waiter ran its own load: %d calls", n)
		}
		close(release)
	})
}
