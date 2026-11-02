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
)

func TestLockRunsOneHolderAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		var running, peak atomic.Int32
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				unlock, err := c.Lock(ctx, "job", time.Minute)
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = unlock(ctx) }()
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
		unlock, _ := c.Lock(ctx, "job", time.Hour)
		go func() {
			time.Sleep(time.Minute)
			_ = unlock(ctx)
		}()
		start := time.Now()
		unlock2, err := c.Lock(ctx, "job", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if waited := time.Since(start); waited < time.Minute {
			t.Fatalf("second Lock returned after %v, before the release", waited)
		}
		_ = unlock2(ctx)
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
