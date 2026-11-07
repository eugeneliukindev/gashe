//go:build go1.25

package gashe_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestShutdownWaitsForBackgroundReloads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		o := gashe.Policy[int]{TTL: gashe.TTL{For: time.Minute}, Stale: gashe.Stale{WhileRevalidate: time.Hour}}
		release := make(chan struct{})
		var loads atomic.Int32
		load := func(context.Context) (int, error) {
			if loads.Add(1) > 1 {
				<-release // the background reload
			}
			return 1, nil
		}

		_, _ = gashe.GetOrLoad(ctx, c, "k", o, load)
		time.Sleep(2 * time.Minute)
		_, _ = gashe.GetOrLoad(ctx, c, "k", o, load) // stale: starts the background reload
		synctest.Wait()

		short, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := c.Shutdown(short); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Shutdown with a reload running = %v, want DeadlineExceeded", err)
		}
		var done atomic.Bool
		go func() {
			_ = c.Shutdown(ctx)
			done.Store(true)
		}()
		synctest.Wait()
		if done.Load() {
			t.Fatal("Shutdown returned while a reload was running")
		}
		close(release)
		synctest.Wait()
		if !done.Load() {
			t.Fatal("Shutdown did not return after the reload finished")
		}

		time.Sleep(2 * time.Minute) // stale again
		if got, err := gashe.GetOrLoad(ctx, c, "k", o, load); err != nil || got != 1 {
			t.Fatalf("after Shutdown: %d, %v; want the stale result", got, err)
		}
		synctest.Wait()
		if n := loads.Load(); n != 2 {
			t.Fatalf("loads = %d; no reload may start after Shutdown", n)
		}
	})
}
