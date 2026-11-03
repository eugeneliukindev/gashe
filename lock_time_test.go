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

func TestLockRunsOneHolderAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		var running, peak atomic.Int32
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				lease, err := c.Lock(ctx, "job", time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = lease.Unlock(ctx) }()
				peak.Store(max(peak.Load(), running.Add(1)))
				time.Sleep(time.Second)
				running.Add(-1)
			})
		}
		wg.Wait()
		if peak.Load() != 1 {
			t.Fatalf("%d holders at once, want 1", peak.Load())
		}
	})
}

func TestLockWaitsForRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		lease, _ := c.Lock(ctx, "job", time.Hour)
		go func() {
			time.Sleep(time.Minute)
			_ = lease.Unlock(ctx)
		}()
		start := time.Now()
		lease2, err := c.Lock(ctx, "job", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if waited := time.Since(start); waited < time.Minute {
			t.Fatalf("second Lock returned after %v, before the release", waited)
		}
		_ = lease2.Unlock(ctx)
	})
}

func TestLockStopsOnContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newCache(t)
		_, _, _ = c.TryLock(context.Background(), "job", time.Hour)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := c.Lock(ctx, "job", time.Hour); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want the deadline", err)
		}
	})
}

func TestLockPollInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		for _, tc := range []struct {
			poll, want time.Duration
		}{
			{0, 200 * time.Millisecond},                     // the default 100ms: checks at 100ms and 200ms
			{time.Second, time.Second},                      // one check a second
			{20 * time.Millisecond, 160 * time.Millisecond}, // the first check after the release
		} {
			c := gashe.New(newCache(t).Backend(), gashe.Options{LockPollInterval: tc.poll})
			held, _ := c.Lock(ctx, "job", time.Hour)
			go func() {
				time.Sleep(150 * time.Millisecond)
				_ = held.Unlock(ctx)
			}()
			start := time.Now()
			lease, err := c.Lock(ctx, "job", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			if waited := time.Since(start); waited != tc.want {
				t.Errorf("LockPollInterval %v: took the lock after %v, want %v", tc.poll, waited, tc.want)
			}
			_ = lease.Unlock(ctx)
		}
	})
}

func TestWithLockExtendsWhileFnRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		err := c.WithLock(ctx, "job", time.Minute, func(ctx context.Context) error {
			time.Sleep(3 * time.Minute) // three times the ttl
			if _, ok, _ := c.TryLock(ctx, "job", time.Minute); ok {
				t.Error("the lock expired while fn ran")
			}
			return ctx.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
		if locked, _ := c.IsLocked(ctx, "job"); locked {
			t.Fatal("WithLock did not release the lock")
		}
	})
}

func TestWithLockCancelsFnWhenTheLockIsLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		err := c.WithLock(ctx, "job", time.Minute, func(ctx context.Context) error {
			_, _ = c.Delete(ctx, "job") // the lock expires or is taken away
			<-ctx.Done()
			return nil
		})
		if !errors.Is(err, gashe.ErrLockLost) {
			t.Fatalf("err = %v, want ErrLockLost", err)
		}
	})
}
