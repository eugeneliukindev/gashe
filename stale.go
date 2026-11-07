package gashe

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

const (
	// maxRefreshLock bounds the refresh lock and the background reload that holds it, so a
	// reload that hangs or a refresher that dies blocks refreshes for at most this long.
	maxRefreshLock = time.Minute
)

// reloadStale reloads an entry past its fresh time; Stale.IfError and Stale.IfSlow may answer
// with the entry instead, each within its own window.
func (l *loading[V]) reloadStale(ctx context.Context, e entry[V], now time.Time) (V, Result, error) {
	ifError := now.Before(e.At.Add(l.o.Stale.IfError.For))
	ifSlow := l.o.Stale.IfSlow.After > 0 && now.Before(e.At.Add(l.o.Stale.IfSlow.For))
	l.stale = ifError || ifSlow
	var v V
	var err error
	if ifSlow {
		var slow bool
		if v, slow, err = l.loadWithin(ctx); slow {
			return e.V, ResultStaleIfSlow, nil
		}
	} else {
		v, err = l.loadAndStore(ctx)
	}
	if r, ok := l.staleOverloaded(ctx, err, ifSlow); ok {
		return e.V, r, nil
	}
	// "Not found" is an answer, stored by loadAndStore: the old result must not hide it.
	if err != nil && ifError && !errors.Is(err, ErrNotFound) && l.o.Stale.IfError.failure(err) {
		l.c.onError(ctx, fmt.Errorf("gashe: %q answered with an old result: %w", l.key, err))
		return e.V, ResultStaleIfError, nil
	}
	return v, ResultMiss, err
}

// refreshInBackground reloads the key in the background unless another caller already holds the
// refresh lock or the cache is shutting down. The reload gets the lock's duration as its deadline.
func (l *loading[V]) refreshInBackground(ctx context.Context) {
	if !l.c.bg.start() {
		return // the cache is shutting down
	}
	ttl := min(l.o.Stale.WhileRevalidate, maxRefreshLock)
	lease, ok, err := l.c.TryLock(ctx, ownRefresh+l.key, ttl)
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: refresh lock %q: %w", l.key, err))
	}
	if !ok {
		l.c.bg.wg.Done()
		return
	}
	r := *l        // the call returns before the reload ends: the reload times itself
	r.stale = true // the old result was answered: a full Stampede.MaxLoads skips the reload
	go func() {
		defer r.c.bg.wg.Done()
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), ttl)
		defer cancel()
		defer r.unlock(bg, lease)
		if _, err := r.loadAndStore(bg); err != nil {
			r.c.onError(bg, fmt.Errorf("gashe: refresh %q: %w", r.key, err))
		}
	}()
}

// loadWithin loads and stores while an old result can answer, waiting at most Stale.IfSlow.After; slow
// reports that the caller should answer with the old result. The load holds the refresh lock and
// runs detached from ctx, so it can finish after the call; a key already loading answers slow at
// once, so a slow source does not get one more load per call.
func (l *loading[V]) loadWithin(ctx context.Context) (v V, slow bool, err error) {
	lease, ok, lockErr := l.c.TryLock(ctx, ownRefresh+l.key, maxRefreshLock)
	switch {
	case lockErr != nil:
		l.c.onError(ctx, fmt.Errorf("gashe: refresh lock %q: %w", l.key, lockErr))
	case !ok:
		return v, true, nil
	}
	if !l.c.bg.start() { // shutting down: no background work, wait for the loader
		defer l.unlock(ctx, lease)
		v, err = l.loadAndStore(ctx)
		return v, false, err
	}
	type result struct {
		v    V
		err  error
		took time.Duration
	}
	done := make(chan result, 1)
	var answered atomic.Bool // the caller stopped waiting
	r := *l
	go func() {
		defer r.c.bg.wg.Done()
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), maxRefreshLock)
		defer cancel()
		defer r.unlock(bg, lease)
		got, loadErr := r.loadAndStore(bg)
		if loadErr != nil && answered.Load() {
			r.c.onError(bg, fmt.Errorf("gashe: slow load %q: %w", r.key, loadErr))
		}
		done <- result{v: got, err: loadErr, took: r.took}
	}()
	select {
	case res := <-done:
		l.took += res.took
		return res.v, false, res.err
	case <-time.After(l.o.Stale.IfSlow.After):
		answered.Store(true)
		return v, true, nil
	case <-ctx.Done():
		answered.Store(true)
		return v, false, ctx.Err()
	}
}

// failure reports whether err, a loader error, is a failure the stale result may answer.
func (s StaleIfError) failure(err error) bool { return s.IsFailure == nil || s.IsFailure(err) }

// staleOverloaded reports whether the stale result answers err, a full Stampede.MaxLoads: it is no
// loader error, so the stale result of any window answers it, whatever IsFailure says.
func (l *loading[V]) staleOverloaded(ctx context.Context, err error, ifSlow bool) (Result, bool) {
	if !errors.Is(err, ErrOverloaded) || !l.stale {
		return 0, false
	}
	l.c.onError(ctx, fmt.Errorf("gashe: %q answered with an old result: %w", l.key, err))
	if ifSlow {
		return ResultStaleIfSlow, true
	}
	return ResultStaleIfError, true
}
