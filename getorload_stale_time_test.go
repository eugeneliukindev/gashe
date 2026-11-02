//go:build go1.25

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

func TestGetOrLoadStaleIfError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newCached(newCache(t), gashe.Policy[int]{TTL: 7 * time.Minute, StaleIfError: 3 * time.Minute})
		calls, load := versions()
		failing := func(context.Context) (int, error) { return 0, errAPI }

		_, _ = s.Do(ctx, "k", load)
		time.Sleep(6 * time.Minute)
		if got, _ := s.Do(ctx, "k", failing); got != 1 {
			t.Fatalf("fresh: v%d; the loader must not run", got)
		}
		time.Sleep(2 * time.Minute) // stale, within StaleIfError
		if got, err := s.Do(ctx, "k", failing); err != nil || got != 1 {
			t.Fatalf("stale with a failing loader: %d, %v; want the stale v1", got, err)
		}
		if got, _ := s.Do(ctx, "k", load); got != 2 || calls.Load() != 2 {
			t.Fatalf("stale with a working loader: v%d; want a reload", got)
		}
		time.Sleep(11 * time.Minute) // past TTL and StaleIfError
		if _, err := s.Do(ctx, "k", failing); !errors.Is(err, errAPI) {
			t.Fatalf("expired: %v, want the loader error", err)
		}
	})
}

func TestGetOrLoadStaleWhileRevalidate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newCached(newCache(t), gashe.Policy[int]{
			TTL:                  7 * time.Minute,
			StaleWhileRevalidate: 3 * time.Minute,
		})
		calls, load := versions()

		_, _ = s.Do(ctx, "k", load)
		time.Sleep(8 * time.Minute)
		if got, _ := s.Do(ctx, "k", load); got != 1 {
			t.Fatalf("stale: v%d, want the stored v1 at once", got)
		}
		synctest.Wait() // the background refresh finishes
		if got, _ := s.Do(ctx, "k", load); got != 2 || calls.Load() != 2 {
			t.Fatalf("after the refresh: v%d, %d calls", got, calls.Load())
		}
		time.Sleep(11 * time.Minute) // past TTL and the window: a plain reload
		if got, _ := s.Do(ctx, "k", load); got != 3 {
			t.Fatalf("expired: v%d, want an inline reload to v3", got)
		}
	})
}

func TestGetOrLoadRevalidatesOnceAcrossProcesses(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		opts := gashe.Policy[int]{TTL: time.Minute, StaleWhileRevalidate: time.Hour}
		// Two strategy values over one cache stand for two processes: no in-process sharing.
		a, b := newCached(c, opts), newCached(c, opts)
		var calls atomic.Int32
		slow := func(context.Context) (int, error) {
			time.Sleep(time.Second)
			return int(calls.Add(1)), nil
		}

		_, _ = a.Do(ctx, "k", slow)
		time.Sleep(2 * time.Minute)
		var wg sync.WaitGroup
		for _, s := range []cached[int]{a, b, a, b} {
			wg.Go(func() { _, _ = s.Do(ctx, "k", slow) })
		}
		wg.Wait()
		time.Sleep(2 * time.Second) // let the background refresh finish its slow load
		if n := calls.Load(); n != 2 {
			t.Fatalf("loader ran %d times, want one load and one refresh", n)
		}
	})
}

func TestGetOrLoadDynamicMode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		// TTL 0: never fresh; every call answers at once and refreshes in the background.
		s := newCached(newCache(t), gashe.Policy[int]{StaleWhileRevalidate: 24 * time.Hour})
		_, load := versions()
		for i, want := range []int{1, 1, 2, 3} {
			if got, _ := s.Do(ctx, "k", load); got != want {
				t.Fatalf("call %d: v%d, want v%d", i+1, got, want)
			}
			synctest.Wait()
		}
	})
}

func TestGetOrLoadRevalidateThenStaleIfError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newCached(newCache(t), gashe.Policy[int]{
			TTL:                  time.Minute,
			StaleWhileRevalidate: time.Minute,
			StaleIfError:         10 * time.Minute,
		})
		_, load := versions()
		_, _ = s.Do(ctx, "k", load)
		time.Sleep(5 * time.Minute) // past the revalidate window, within StaleIfError
		got, err := s.Do(ctx, "k", func(context.Context) (int, error) { return 0, errAPI })
		if err != nil || got != 1 {
			t.Fatalf("got %d, %v; want the stale v1", got, err)
		}
	})
}

func TestGetOrLoadMissLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		opts := gashe.Policy[int]{TTL: time.Hour, LockOnMiss: 10 * time.Second}
		var calls atomic.Int32
		slow := func(context.Context) (int, error) {
			time.Sleep(time.Second)
			return int(calls.Add(1)), nil
		}

		var wg sync.WaitGroup
		for range 3 {
			s := newCached(c, opts) // a separate process each
			wg.Go(func() {
				if got, err := s.Do(ctx, "k", slow); err != nil || got != 1 {
					t.Errorf("Do = %d, %v; want everyone to get v1", got, err)
				}
			})
		}
		wg.Wait()
		if n := calls.Load(); n != 1 {
			t.Fatalf("loader ran %d times for one miss, want 1", n)
		}
		if ok, _ := c.IsLocked(ctx, "gashe:load:k"); ok {
			t.Fatal("load lock left behind")
		}
	})
}
