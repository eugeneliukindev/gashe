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

// gate is a loader that blocks until it is opened and counts how many run at once.
type gate struct {
	open         chan struct{}
	running, max atomic.Int32
}

func newGate() *gate { return &gate{open: make(chan struct{})} }

func (g *gate) load(ctx context.Context) (user, error) {
	n := g.running.Add(1)
	defer g.running.Add(-1)
	for {
		m := g.max.Load()
		if n <= m || g.max.CompareAndSwap(m, n) {
			break
		}
	}
	select {
	case <-g.open:
		return user{Name: "Ann"}, nil
	case <-ctx.Done():
		return user{}, ctx.Err()
	}
}

func TestMaxConcurrentLoadsHoldsTheLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		users := gashe.For(
			newCache(t),
			gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{MaxLoads: 2}},
		)
		g := newGate()
		var wg sync.WaitGroup
		for _, key := range []string{"1", "2", "3", "4", "5"} {
			wg.Go(func() {
				if _, err := users.GetOrLoad(ctx, key, g.load); err != nil {
					t.Error(err)
				}
			})
		}
		synctest.Wait()
		if g.running.Load() != 2 {
			t.Fatalf("%d loads run at once, want the limit of 2", g.running.Load())
		}
		close(g.open)
		wg.Wait()
		if g.max.Load() != 2 {
			t.Fatalf("at most %d loads ran at once, want 2", g.max.Load())
		}
	})
}

func TestMaxConcurrentLoadsWaitsAsLongAsCtx(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		users := gashe.For(
			newCache(t),
			gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{MaxLoads: 1}},
		)
		g := newGate()
		defer close(g.open)
		go func() { _, _ = users.GetOrLoad(context.Background(), "1", g.load) }()
		synctest.Wait()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := users.GetOrLoad(ctx, "2", g.load)
		if !errors.Is(err, gashe.ErrOverloaded) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want ErrOverloaded after the ctx deadline", err)
		}
	})
}

func TestMaxConcurrentLoadsAnswersAnOldResultAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		users := gashe.For(newCache(t), gashe.Policy[user]{
			TTL:      gashe.TTL{For: time.Minute},
			Stampede: gashe.Stampede{MaxLoads: 1},
			Stale:    gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}},
		})
		_, _ = users.GetOrLoad(ctx, "old", func(context.Context) (user, error) { return user{Name: "old"}, nil })
		time.Sleep(2 * time.Minute) // "old" is past its TTL, within StaleIfError

		g := newGate()
		defer close(g.open)
		go func() { _, _ = users.GetOrLoad(ctx, "busy", g.load) }() // takes the only slot
		synctest.Wait()

		start := time.Now()
		got, err := users.GetOrLoad(ctx, "old", g.load)
		if err != nil || got.Name != "old" || time.Since(start) != 0 {
			t.Fatalf("got %+v, %v after %v; want the old result at once", got, err, time.Since(start))
		}
	})
}

// A full limit answers with the stale result whenever a stale window covers the key: IfSlow alone,
// and IfError even when IsFailure does not count ErrOverloaded as a failure.
func TestMaxConcurrentLoadsAnswersTheStaleResultOfAnyWindow(t *testing.T) {
	for name, stale := range map[string]gashe.Stale{
		"IfSlow":                 {IfSlow: gashe.StaleIfSlow{For: time.Hour, After: time.Second}},
		"IfError, any IsFailure": {IfError: gashe.StaleIfError{For: time.Hour, IsFailure: func(error) bool { return false }}},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				users := gashe.For(
					newCache(t),
					gashe.Policy[user]{
						TTL:      gashe.TTL{For: time.Minute},
						Stampede: gashe.Stampede{MaxLoads: 1},
						Stale:    stale,
					},
				)
				_, _ = users.GetOrLoad(
					ctx,
					"old",
					func(context.Context) (user, error) { return user{Name: "old"}, nil },
				)
				time.Sleep(2 * time.Minute) // "old" is past its TTL, within the window

				g := newGate()
				defer close(g.open)
				go func() { _, _ = users.GetOrLoad(ctx, "busy", g.load) }() // takes the only slot
				synctest.Wait()

				if got, err := users.GetOrLoad(ctx, "old", g.load); err != nil || got.Name != "old" {
					t.Fatalf("got %+v, %v; want the stale result at once", got, err)
				}
			})
		})
	}
}

func TestMaxConcurrentLoadsIsPerKindOfData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		users := gashe.For(
			c,
			gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Prefix: "u:", Stampede: gashe.Stampede{MaxLoads: 1}},
		)
		admins := gashe.For(
			c,
			gashe.Policy[user]{TTL: gashe.TTL{For: time.Hour}, Prefix: "a:", Stampede: gashe.Stampede{MaxLoads: 1}},
		)
		g := newGate()
		defer close(g.open)
		go func() { _, _ = users.GetOrLoad(ctx, "1", g.load) }()
		synctest.Wait()

		if _, err := admins.GetOrLoad(
			ctx,
			"1",
			func(context.Context) (user, error) { return user{}, nil },
		); err != nil {
			t.Fatalf("a full limit of users held back admins: %v", err)
		}
	})
}
