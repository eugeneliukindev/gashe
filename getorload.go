package gashe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"
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
	ttl, err := o.check()
	if err != nil {
		var zero V
		return zero, err
	}
	return getOrLoad(ctx, c, key, o, ttl, load)
}

// getOrLoad is GetOrLoad with o checked; ttl is what Policy.check returned.
func getOrLoad[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	ttl func(V) time.Duration,
	load func(context.Context) (V, error),
) (V, error) {
	l := newLoading(c, o, ttl, key, load)
	run := func(ctx context.Context) (any, error) {
		v, r, getErr := l.get(ctx)
		return answer[V]{v: v, r: r, took: l.took}, getErr
	}
	shared, err := c.calls.do(ctx, groupKey{key: l.key, typ: reflect.TypeFor[V]()}, run)
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
	ttl, err := o.check()
	if err != nil {
		var zero V
		return zero, err
	}
	return reload(ctx, c, key, o, ttl, load)
}

// reload is Reload with o checked; ttl is what Policy.check returned.
func reload[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	ttl func(V) time.Duration,
	load func(context.Context) (V, error),
) (V, error) {
	l := newLoading(c, o, ttl, key, load)
	gens, err := l.readGens(ctx)
	if err != nil { // the backend failed: load, and keep the result in Policy.Fallback
		v, loadErr := l.call(ctx)
		if loadErr == nil {
			l.spareResult(ctx, l.key, v)
		}
		return v, loadErr
	}
	l.gens = gens
	v, err := l.call(ctx)
	if err != nil { // any error, "not found" too, leaves the stored result alone
		return v, err
	}
	return l.storeResult(ctx, v), nil
}

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
	ttl   func(V) time.Duration // o.TTL as checked by Policy.check
	key   string
	load  func(context.Context) (V, error)
	gens  []int64       // current generations of o.Tags, read before loading
	shape uint64        // shapeOf V
	took  time.Duration // how long the loader ran in this call
	stale bool          // an old result can answer, so a full MaxConcurrentLoads fails at once
	spare spare         // Policy.Fallback
}

func newLoading[V any](
	c *Cache,
	o Policy[V],
	ttl func(V) time.Duration,
	key string,
	load func(context.Context) (V, error),
) loading[V] {
	return loading[V]{
		c:     c,
		o:     o,
		ttl:   ttl,
		key:   o.Prefix + key,
		load:  load,
		shape: c.shapeOf(reflect.TypeFor[V]()),
		spare: c.spareOf(o.Fallback),
	}
}

// call runs the loader and adds the time it took to took.
func (l *loading[V]) call(ctx context.Context) (V, error) {
	release, err := l.acquire(ctx)
	if err != nil {
		var zero V
		return zero, err
	}
	defer release()
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
		return l.withoutBackend(ctx)
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
		if l.notFound(err) {
			l.storeNegative(ctx)
		}
		return v, err
	}
	return l.storeResult(ctx, v), nil
}

// notFound reports whether err is an answer Negative caches, not a failure.
func (l *loading[V]) notFound(err error) bool {
	return l.o.Negative.If != nil && l.o.Negative.If(err)
}

// storeResult stores v as the policy says and returns it as it reads back.
func (l *loading[V]) storeResult(ctx context.Context, v V) V {
	if l.o.CacheIf != nil && !l.o.CacheIf(v) {
		return v
	}
	fresh, ok := l.freshFor(v)
	if !ok {
		return v
	}
	return store(
		ctx,
		l.c,
		l.spare,
		l.key,
		entry[V]{At: time.Now().Add(fresh), V: v, G: l.gens, T: l.shape},
		fresh+l.o.keep(),
	).V
}

// freshFor returns the jittered fresh TTL of v, and false if v must not be stored.
func (l *loading[V]) freshFor(v V) (time.Duration, bool) {
	fresh := l.ttl(v)
	if fresh < 0 || fresh+l.o.keep() <= 0 {
		return 0, false
	}
	return l.o.spread(fresh), true
}

// storeNegative stores a negative answer for Negative.TTL.
func (l *loading[V]) storeNegative(ctx context.Context) {
	ttl := l.o.spread(l.o.Negative.TTL)
	store(ctx, l.c, l.spare, l.key, entry[V]{At: time.Now().Add(ttl), Neg: true, G: l.gens, T: l.shape}, ttl)
}

// loadLocked loads a missing key under the load lock and answers with the result of whoever held
// the lock before, if there is one. If the backend cannot lock, it loads without the lock.
func (l *loading[V]) loadLocked(ctx context.Context) (V, Result, error) {
	lease, err := l.c.Lock(ctx, ownLoad+l.key, l.o.LockOnMiss)
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
	if e, readErr := l.read(ctx); readErr == nil {
		switch now := time.Now(); {
		case e.Neg && now.Before(e.At): // the holder before stored "not found"
			var zero V
			return zero, ResultNegativeHit, l.o.Negative.Err
		case !e.Neg && now.Before(e.At.Add(l.o.StaleWhileRevalidate)):
			return e.V, ResultHit, nil
		}
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
