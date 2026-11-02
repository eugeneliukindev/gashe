package gashe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync/atomic"
	"time"
)

const (
	// maxRefreshLock bounds the refresh lock and the background reload that holds it, so a
	// reload that hangs or a refresher that dies blocks refreshes for at most this long.
	maxRefreshLock = time.Minute
)

// GetOrLoad returns the result stored under key, or calls load, stores its result and returns
// it; o says when a stored result is still good enough. Stored results are shared by every
// process that uses the same backend. Concurrent calls with the same key and type share one
// lookup and one load; this sharing lives in the memory of this process.
//
// An error from load is returned as is and is not stored, unless StaleIfError answers it or
// Negative caches it. If the backend fails, GetOrLoad returns the result of load without storing
// it. Errors it works around go to Options.OnError. Invalid options return ErrInvalidOptions.
func GetOrLoad[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	load func(context.Context) (V, error),
) (V, error) {
	if err := o.check(); err != nil {
		var zero V
		return zero, err
	}
	l := loading[V]{c: c, o: o, key: o.Prefix + key, load: load, shape: c.shapeOf(reflect.TypeFor[V]())}
	shared, err := c.calls.do(ctx, groupKey{key: l.key, typ: reflect.TypeFor[V]()}, func() (any, error) {
		v, r, err := l.get(ctx)
		return answer[V]{v: v, r: r, took: l.took}, err
	})
	a, _ := shared.(answer[V]) // empty when the wait was cancelled
	if a.r != 0 {
		c.onResult(ctx, Outcome{Key: l.key, Result: a.r, Load: a.took})
	}
	return a.v, err
}

// Reload calls load and stores its result under key regardless of what is stored, for example
// after the data behind key changed. An error from load is returned and leaves the stored result
// alone.
func Reload[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	load func(context.Context) (V, error),
) (V, error) {
	if err := o.check(); err != nil {
		var zero V
		return zero, err
	}
	l := loading[V]{c: c, o: o, key: o.Prefix + key, load: load, shape: c.shapeOf(reflect.TypeFor[V]())}
	gens, err := l.readGens(ctx)
	if err != nil {
		return load(ctx)
	}
	l.gens = gens
	return l.loadAndStore(ctx)
}

// entry is what GetOrLoad stores: the result, when it stops being fresh, the generations of its
// tags, and whether it is a negative answer.
type entry[V any] struct {
	At  time.Time `json:"at"`
	V   V         `json:"v"`
	Neg bool      `json:"neg,omitempty"`
	G   []int64   `json:"g,omitempty"`
	T   uint64    `json:"t,omitempty"`
}

// sameShape reports whether the entry was stored with the shape of the current V. Entries
// stored before shapes were kept have none and are taken as they are.
func (e *entry[V]) sameShape(shape uint64) bool { return e.T == 0 || e.T == shape }

// answer is what concurrent callers of one GetOrLoad share.
type answer[V any] struct {
	v    V
	r    Result
	took time.Duration
}

// loading is one GetOrLoad or Reload call.
type loading[V any] struct {
	c     *Cache
	o     Policy[V]
	key   string
	load  func(context.Context) (V, error)
	gens  []int64       // current generations of o.Tags, read before loading
	shape uint64        // shapeOf V
	took  time.Duration // how long the loader ran in this call
}

// call runs the loader and adds the time it took to took.
func (l *loading[V]) call(ctx context.Context) (V, error) {
	start := time.Now()
	v, err := l.load(ctx)
	l.took += time.Since(start)
	return v, err
}

func (l *loading[V]) get(ctx context.Context) (V, Result, error) {
	e, err := l.read(ctx)
	if err == nil && e.Neg && !time.Now().Before(e.At) {
		err = ErrNotFound // an expired negative answer is not a fallback
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return l.miss(ctx)
	case err != nil:
		v, loadErr := l.call(ctx) // the backend failed: answer without storing
		return v, ResultMiss, loadErr
	}
	return l.answer(ctx, e)
}

// miss loads a key that has no usable entry.
func (l *loading[V]) miss(ctx context.Context) (V, Result, error) {
	if l.o.LockOnMiss > 0 {
		return l.loadLocked(ctx)
	}
	v, err := l.loadAndStore(ctx)
	return v, ResultMiss, err
}

// answer answers from a stored entry: fresh, old within a window, or reloaded.
func (l *loading[V]) answer(ctx context.Context, e entry[V]) (V, Result, error) {
	now := time.Now()
	switch {
	case e.Neg:
		var zero V
		return zero, ResultNegativeHit, l.o.Negative.Err
	case now.Before(e.At):
		return e.V, ResultHit, nil
	case now.Before(e.At.Add(l.o.StaleWhileRevalidate)):
		l.refreshInBackground(ctx)
		return e.V, ResultStaleWhileRevalidate, nil
	}
	return l.reloadStale(ctx, e, now)
}

// reloadStale reloads an entry past its fresh time; StaleIfError and StaleIfSlow may answer
// with the entry instead.
func (l *loading[V]) reloadStale(ctx context.Context, e entry[V], now time.Time) (V, Result, error) {
	staleOK := now.Before(e.At.Add(l.o.StaleIfError))
	var v V
	var err error
	if staleOK && l.o.StaleIfSlow > 0 {
		var slow bool
		if v, slow, err = l.loadWithin(ctx); slow {
			return e.V, ResultStaleIfSlow, nil
		}
	} else {
		v, err = l.loadAndStore(ctx)
	}
	if err != nil && staleOK && (l.o.StaleIf == nil || l.o.StaleIf(err)) {
		l.c.onError(ctx, fmt.Errorf("gashe: %q answered with an old result: %w", l.key, err))
		return e.V, ResultStaleIfError, nil
	}
	return v, ResultMiss, err
}

// read returns the stored entry under the key and keeps the current generations of its tags.
// ErrNotFound means a miss, including an entry of an older generation or of another shape of V;
// any other error is a backend failure and is already reported.
func (l *loading[V]) read(ctx context.Context) (entry[V], error) {
	e, err := l.readEntry(ctx)
	if err == nil && !e.sameShape(l.shape) {
		return e, ErrNotFound // V changed since the entry was stored
	}
	return e, err
}

func (l *loading[V]) readEntry(ctx context.Context) (entry[V], error) {
	if len(l.o.Tags) == 0 {
		return lookup[entry[V]](ctx, l.c, l.key)
	}
	var e entry[V]
	keys := append([]string{l.key}, l.tagKeys()...)
	raws, err := l.c.backend.GetMany(ctx, keys...)
	if err != nil {
		err = fmt.Errorf("gashe: get %q: %w", l.key, err)
		l.c.onError(ctx, err)
		return e, err
	}
	if l.gens, err = l.c.generations(ctx, keys[1:], raws[1:]); err != nil {
		err = fmt.Errorf("gashe: get %q: %w", l.key, err)
		l.c.onError(ctx, err)
		return e, err
	}
	if raws[0] == nil {
		return e, ErrNotFound
	}
	if err := l.c.decode(l.key, raws[0], &e); err != nil {
		l.c.onError(ctx, err)
		return e, ErrNotFound
	}
	if !slices.Equal(e.G, l.gens) {
		return e, ErrNotFound // a tag was invalidated after the entry was stored
	}
	return e, nil
}

// loadAndStore calls the loader and stores its result if it qualifies.
func (l *loading[V]) loadAndStore(ctx context.Context) (V, error) {
	v, err := l.call(ctx)
	if err != nil {
		if l.o.Negative.If != nil && l.o.Negative.If(err) {
			l.storeNegative(ctx)
		}
		return v, err
	}
	if l.o.CacheIf != nil && !l.o.CacheIf(v) {
		return v, nil
	}
	fresh, ok := l.freshFor(v)
	if !ok {
		return v, nil
	}
	return store(
		ctx,
		l.c,
		l.key,
		entry[V]{At: time.Now().Add(fresh), V: v, G: l.gens, T: l.shape},
		fresh+l.o.keep(),
	).V, nil
}

// freshFor returns the jittered fresh TTL of v, and false if v must not be stored.
func (l *loading[V]) freshFor(v V) (time.Duration, bool) {
	fresh := l.o.TTL
	if l.o.TTLFunc != nil {
		fresh = l.o.TTLFunc(v)
	}
	if fresh < 0 || fresh+l.o.keep() <= 0 {
		return 0, false
	}
	return l.o.spread(fresh), true
}

// storeNegative stores a negative answer for Negative.TTL.
func (l *loading[V]) storeNegative(ctx context.Context) {
	ttl := l.o.spread(l.o.Negative.TTL)
	store(ctx, l.c, l.key, entry[V]{At: time.Now().Add(ttl), Neg: true, G: l.gens, T: l.shape}, ttl)
}

// refreshInBackground reloads the key in the background unless another caller already holds the
// refresh lock or the cache is shutting down. The reload gets the lock's duration as its deadline.
func (l *loading[V]) refreshInBackground(ctx context.Context) {
	if !l.c.bg.start() {
		return // the cache is shutting down
	}
	ttl := min(l.o.StaleWhileRevalidate, maxRefreshLock)
	lease, ok, err := l.c.TryLock(ctx, l.c.ownKey(spaceRefresh, l.key), ttl)
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: refresh lock %q: %w", l.key, err))
	}
	if !ok {
		l.c.bg.wg.Done()
		return
	}
	r := *l // the call returns before the reload ends: the reload times itself
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

// loadWithin loads and stores while an old result can answer, waiting at most StaleIfSlow; slow
// reports that the caller should answer with the old result. The load holds the refresh lock and
// runs detached from ctx, so it can finish after the call; a key already loading answers slow at
// once, so a slow source does not get one more load per call.
func (l *loading[V]) loadWithin(ctx context.Context) (v V, slow bool, err error) {
	lease, ok, lockErr := l.c.TryLock(ctx, l.c.ownKey(spaceRefresh, l.key), maxRefreshLock)
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
	case <-time.After(l.o.StaleIfSlow):
		answered.Store(true)
		return v, true, nil
	case <-ctx.Done():
		answered.Store(true)
		return v, false, ctx.Err()
	}
}

// loadLocked loads a missing key under the load lock and answers with the result of whoever held
// the lock before, if there is one. If the backend cannot lock, it loads without the lock.
func (l *loading[V]) loadLocked(ctx context.Context) (V, Result, error) {
	lease, err := l.c.Lock(ctx, l.c.ownKey(spaceLoad, l.key), l.o.LockOnMiss)
	if err != nil {
		if ctx.Err() != nil {
			var zero V
			return zero, 0, err
		}
		l.c.onError(ctx, fmt.Errorf("gashe: load lock %q: %w", l.key, err))
		v, loadErr := l.loadAndStore(ctx)
		return v, ResultMiss, loadErr
	}
	defer l.unlock(ctx, lease)
	if e, readErr := l.read(ctx); readErr == nil && !e.Neg && time.Now().Before(e.At.Add(l.o.StaleWhileRevalidate)) {
		return e.V, ResultHit, nil
	}
	v, err := l.loadAndStore(ctx)
	return v, ResultMiss, err
}

// unlock releases lease; nil means the lock could not be taken, so there is nothing to release.
func (l *loading[V]) unlock(ctx context.Context, lease *Lease) {
	if lease == nil {
		return
	}
	if err := lease.Unlock(ctx); err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: unlock %q: %w", lease.key, err))
	}
}

// lookup returns the decoded value under key. ErrNotFound means a miss; an undecodable entry
// counts as a miss too. Any other error is a backend failure and is already reported.
func lookup[V any](ctx context.Context, c *Cache, key string) (V, error) {
	var v V
	raw, err := c.backend.Get(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return v, ErrNotFound
	}
	if err != nil {
		err = fmt.Errorf("gashe: get %q: %w", key, err)
		c.onError(ctx, err)
		return v, err
	}
	if err := c.decode(key, raw, &v); err != nil {
		c.onError(ctx, err)
		return v, ErrNotFound
	}
	return v, nil
}

// store saves v under key and returns v as it reads back from the cache, so callers see the
// same value on a hit and on a miss. Failures are reported and leave v as it is.
func store[V any](ctx context.Context, c *Cache, key string, v V, ttl time.Duration) V {
	raw, err := c.encode(key, v, ttl)
	if err != nil {
		c.onError(ctx, err)
		return v
	}
	var out V
	if err := c.decode(key, raw, &out); err != nil {
		c.onError(ctx, err)
		return v
	}
	if err := c.backend.Set(ctx, key, raw, ttl); err != nil {
		c.onError(ctx, fmt.Errorf("gashe: set %q: %w", key, err))
	}
	return out
}
