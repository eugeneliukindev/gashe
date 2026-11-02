package gashe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"
)

const (
	refreshLockSuffix = ":refresh"
	loadLockSuffix    = ":load"
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
	l := loading[V]{c: c, o: o, key: o.Prefix + key, load: load}
	shared, err := c.calls.do(ctx, groupKey{key: l.key, typ: reflect.TypeFor[V]()}, func() (any, error) {
		v, r, err := l.get(ctx)
		return answer[V]{v: v, r: r}, err
	})
	a, _ := shared.(answer[V]) // empty when the wait was cancelled
	if a.r != 0 {
		c.onResult(ctx, l.key, a.r)
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
	l := loading[V]{c: c, o: o, key: o.Prefix + key, load: load}
	gens, err := l.readGens(ctx)
	if err != nil {
		return load(ctx)
	}
	l.gens = gens
	return l.loadAndStore(ctx, true)
}

// entry is what GetOrLoad stores: the result, when it stops being fresh, the generations of its
// tags, and whether it is a negative answer.
type entry[V any] struct {
	At  time.Time `json:"at"`
	V   V         `json:"v"`
	Neg bool      `json:"neg,omitempty"`
	G   []int64   `json:"g,omitempty"`
}

// answer is what concurrent callers of one GetOrLoad share.
type answer[V any] struct {
	v V
	r Result
}

// loading is one GetOrLoad or Reload call.
type loading[V any] struct {
	c    *Cache
	o    Policy[V]
	key  string
	load func(context.Context) (V, error)
	gens []int64 // current generations of o.Tags, read before loading
}

func (l *loading[V]) get(ctx context.Context) (V, Result, error) {
	e, err := l.read(ctx)
	if err == nil && e.Neg && !time.Now().Before(e.At) {
		err = ErrNotFound // an expired negative answer is not a fallback
	}
	if err != nil {
		return l.miss(ctx, errors.Is(err, ErrNotFound))
	}
	return l.answer(ctx, e)
}

// miss loads a key that has no usable entry; backendOK is false when the backend failed.
func (l *loading[V]) miss(ctx context.Context, backendOK bool) (V, Result, error) {
	if backendOK && l.o.LockOnMiss > 0 {
		return l.loadLocked(ctx)
	}
	v, err := l.loadAndStore(ctx, backendOK)
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
	v, err := l.loadAndStore(ctx, true)
	if err != nil && now.Before(e.At.Add(l.o.StaleIfError)) && (l.o.StaleIf == nil || l.o.StaleIf(err)) {
		l.c.onError(ctx, fmt.Errorf("gashe: %q answered with an old result: %w", l.key, err))
		return e.V, ResultStaleIfError, nil
	}
	return v, ResultMiss, err
}

// read returns the stored entry under the key and keeps the current generations of its tags.
// ErrNotFound means a miss, including an entry of an older generation; any other error is a
// backend failure and is already reported.
func (l *loading[V]) read(ctx context.Context) (entry[V], error) {
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
	l.gens = generations(raws[1:])
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

// loadAndStore calls the loader and, if the backend is fine and the result qualifies, stores it.
func (l *loading[V]) loadAndStore(ctx context.Context, backendOK bool) (V, error) {
	v, err := l.load(ctx)
	if err != nil {
		if backendOK && l.o.Negative.If != nil && l.o.Negative.If(err) {
			l.storeNegative(ctx)
		}
		return v, err
	}
	if !backendOK || (l.o.CacheIf != nil && !l.o.CacheIf(v)) {
		return v, nil
	}
	fresh, ok := l.freshFor(v)
	if !ok {
		return v, nil
	}
	return store(ctx, l.c, l.key, entry[V]{At: time.Now().Add(fresh), V: v, G: l.gens}, fresh+l.o.keep()).V, nil
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
	store(ctx, l.c, l.key, entry[V]{At: time.Now().Add(ttl), Neg: true, G: l.gens}, ttl)
}

// refreshInBackground reloads the key in the background unless another caller already holds the
// refresh lock. The reload gets the lock's duration as its deadline.
func (l *loading[V]) refreshInBackground(ctx context.Context) {
	ttl := min(l.o.StaleWhileRevalidate, maxRefreshLock)
	lease, ok, err := l.c.TryLock(ctx, l.key+refreshLockSuffix, ttl)
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: refresh lock %q: %w", l.key, err))
	}
	if !ok {
		return
	}
	go func() {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), ttl)
		defer cancel()
		defer l.unlock(bg, lease)
		if _, err := l.loadAndStore(bg, true); err != nil {
			l.c.onError(bg, fmt.Errorf("gashe: refresh %q: %w", l.key, err))
		}
	}()
}

// loadLocked loads a missing key under the load lock and answers with the result of whoever held
// the lock before, if there is one. If the backend cannot lock, it loads without the lock.
func (l *loading[V]) loadLocked(ctx context.Context) (V, Result, error) {
	lease, err := l.c.Lock(ctx, l.key+loadLockSuffix, l.o.LockOnMiss)
	if err != nil {
		if ctx.Err() != nil {
			var zero V
			return zero, 0, err
		}
		l.c.onError(ctx, fmt.Errorf("gashe: load lock %q: %w", l.key, err))
		v, loadErr := l.loadAndStore(ctx, true)
		return v, ResultMiss, loadErr
	}
	defer l.unlock(ctx, lease)
	if e, readErr := l.read(ctx); readErr == nil && !e.Neg && time.Now().Before(e.At.Add(l.o.StaleWhileRevalidate)) {
		return e.V, ResultHit, nil
	}
	v, err := l.loadAndStore(ctx, true)
	return v, ResultMiss, err
}

func (l *loading[V]) unlock(ctx context.Context, lease *Lease) {
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
