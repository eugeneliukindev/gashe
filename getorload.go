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
	readsSuffix       = ":reads"
	// maxRefreshLock bounds the refresh lock, so a refresher that dies blocks refreshes for at
	// most this long; a refresh that runs longer may run twice.
	maxRefreshLock = time.Minute
)

// CacheOptions says how GetOrLoad caches results of type V. Define it once and pass it to
// every call; the zero value of each field means "off".
//
// A stored result is fresh for TTL: it is answered without calling the loader. After TTL it is
// old: RefreshWindow and FallbackWindow say for how much longer it is still good enough. The
// entry is kept for TTL plus the longer of the two windows.
type CacheOptions[V any] struct {
	// TTL is how long a result is fresh. It may be 0 when a window is set: then every call
	// either refreshes in the background (RefreshWindow) or calls the loader (FallbackWindow).
	TTL time.Duration
	// TTLFunc, when set, picks TTL from the result. A negative TTL means "do not store".
	TTLFunc func(V) time.Duration
	// RefreshWindow is how long after TTL an old result is still answered at once, while one
	// caller across all processes reloads it in the background.
	RefreshWindow time.Duration
	// FallbackWindow is how long after TTL an old result answers a loader error, so callers keep
	// getting data while the source is down.
	FallbackWindow time.Duration
	// FallbackIf decides which loader errors FallbackWindow covers. nil means any error.
	FallbackIf func(error) bool
	// LockOnMiss, when set, makes a missing key load under a lock: one process calls the loader,
	// the others wait and take its result. The value is the longest the lock is held.
	LockOnMiss time.Duration
	// CacheIf decides whether a result is stored. nil stores every successful result.
	CacheIf func(V) bool
	// MinLoadTime stores a result only if the loader took longer than this.
	MinLoadTime time.Duration
	// MaxReads, when set, makes a result load again after this many reads, even while fresh.
	// It needs a TTL.
	MaxReads int
	// RefreshAfterReads, when set, reloads the result in the background on that read while still
	// answering with it. It needs MaxReads.
	RefreshAfterReads int
	// Prefix is added to every key as "prefix:key".
	Prefix string
}

// GetOrLoad returns the result stored under key, or calls load, stores its result and returns
// it; o says when a stored result is still good enough. Concurrent calls with the same key and
// type inside the process share one lookup and one load.
//
// An error from load is returned as is and nothing is stored, unless FallbackWindow answers it.
// If the backend fails, GetOrLoad returns the result of load without storing it. Errors it works
// around go to Options.OnError. Invalid options return ErrInvalidOptions.
func GetOrLoad[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o CacheOptions[V],
	load func(context.Context) (V, error),
) (V, error) {
	if err := o.check(); err != nil {
		var zero V
		return zero, err
	}
	l := loading[V]{c: c, o: o, key: prefixed(o.Prefix, key), load: load}
	v, err := c.calls.do(ctx, groupKey{key: l.key, typ: reflect.TypeFor[V]()}, func() (any, error) {
		return l.get(ctx)
	})
	out, _ := v.(V) // v is nil when the loader returned a nil V or the wait was cancelled
	return out, err
}

// Reload calls load and stores its result under key regardless of what is stored, for example
// after the data behind key changed. An error from load is returned and leaves the stored result
// alone.
func Reload[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o CacheOptions[V],
	load func(context.Context) (V, error),
) (V, error) {
	if err := o.check(); err != nil {
		var zero V
		return zero, err
	}
	l := loading[V]{c: c, o: o, key: prefixed(o.Prefix, key), load: load}
	return l.loadAndStore(ctx, true)
}

func (o CacheOptions[V]) check() error {
	for _, rule := range []struct {
		broken bool
		why    string
	}{
		{negative(o.TTL, o.RefreshWindow, o.FallbackWindow, o.LockOnMiss, o.MinLoadTime), "durations must not be negative"},
		{o.MaxReads < 0 || o.RefreshAfterReads < 0, "MaxReads and RefreshAfterReads must not be negative"},
		{o.TTL == 0 && o.TTLFunc == nil && o.keep() == 0, "set TTL, TTLFunc or a window, or nothing is ever stored"},
		{o.MaxReads > 0 && o.TTL == 0, "MaxReads needs a TTL"},
		{o.RefreshAfterReads > 0 && o.MaxReads == 0, "RefreshAfterReads needs MaxReads"},
	} {
		if rule.broken {
			return fmt.Errorf("%w: %s", ErrInvalidOptions, rule.why)
		}
	}
	return nil
}

func negative(ds ...time.Duration) bool {
	return slices.ContainsFunc(ds, func(d time.Duration) bool { return d < 0 })
}

// keep is how long an entry outlives its fresh period.
func (o CacheOptions[V]) keep() time.Duration { return max(o.RefreshWindow, o.FallbackWindow) }

// loading is one GetOrLoad or Reload call.
type loading[V any] struct {
	c    *Cache
	o    CacheOptions[V]
	key  string
	load func(context.Context) (V, error)
}

func (l loading[V]) get(ctx context.Context) (V, error) {
	entry, err := lookup[stamped[V]](ctx, l.c, l.key)
	if err != nil {
		backendOK := errors.Is(err, ErrNotFound)
		if backendOK && l.o.LockOnMiss > 0 {
			return l.loadLocked(ctx)
		}
		return l.loadAndStore(ctx, backendOK)
	}
	now := time.Now()
	old := !now.Before(entry.At)
	if !old && !l.readsUsedUp(ctx) {
		return entry.V, nil
	}
	if old && now.Before(entry.At.Add(l.o.RefreshWindow)) {
		l.refreshInBackground(ctx)
		return entry.V, nil
	}
	v, err := l.loadAndStore(ctx, true)
	if err != nil && l.fallsBack(err, entry, now) {
		l.c.onError(ctx, fmt.Errorf("gashe: %q answered with an old result: %w", l.key, err))
		return entry.V, nil
	}
	return v, err
}

// fallsBack reports whether a loader error at now may be answered with the old entry.
func (l loading[V]) fallsBack(err error, entry stamped[V], now time.Time) bool {
	return now.Before(entry.At.Add(l.o.FallbackWindow)) && (l.o.FallbackIf == nil || l.o.FallbackIf(err))
}

// loadAndStore calls the loader and, if the backend is fine and the result qualifies, stores it.
func (l loading[V]) loadAndStore(ctx context.Context, backendOK bool) (V, error) {
	v, elapsed, err := timedLoad(ctx, l.load)
	if err != nil || !backendOK || !l.worthStoring(v, elapsed) {
		return v, err
	}
	fresh := l.o.TTL
	if l.o.TTLFunc != nil {
		fresh = l.o.TTLFunc(v)
	}
	if fresh < 0 || fresh+l.o.keep() <= 0 {
		return v, nil
	}
	out := store(ctx, l.c, l.key, stamped[V]{At: time.Now().Add(fresh), V: v}, fresh+l.o.keep()).V
	if l.o.MaxReads > 0 {
		if _, err := l.c.backend.Delete(ctx, l.key+readsSuffix); err != nil {
			l.c.onError(ctx, fmt.Errorf("gashe: delete %q: %w", l.key+readsSuffix, err))
		}
	}
	return out, nil
}

func (l loading[V]) worthStoring(v V, elapsed time.Duration) bool {
	if l.o.MinLoadTime > 0 && elapsed <= l.o.MinLoadTime {
		return false
	}
	return l.o.CacheIf == nil || l.o.CacheIf(v)
}

// readsUsedUp counts this read and reports whether the result has served MaxReads reads.
// On read number RefreshAfterReads it starts a background refresh.
func (l loading[V]) readsUsedUp(ctx context.Context) bool {
	if l.o.MaxReads == 0 {
		return false
	}
	reads, err := l.c.backend.Incr(ctx, l.key+readsSuffix, 1, l.o.TTL+l.o.keep())
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: incr %q: %w", l.key+readsSuffix, err))
		return false
	}
	if reads == int64(l.o.RefreshAfterReads) {
		l.refreshInBackground(ctx)
	}
	return reads > int64(l.o.MaxReads)
}

// refreshInBackground reloads the key in the background unless another caller already holds the
// refresh lock.
func (l loading[V]) refreshInBackground(ctx context.Context) {
	ttl := maxRefreshLock
	if l.o.RefreshWindow > 0 {
		ttl = min(l.o.RefreshWindow, maxRefreshLock)
	}
	unlock, ok, err := l.c.TryLock(ctx, l.key+refreshLockSuffix, ttl)
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: refresh lock %q: %w", l.key, err))
	}
	if !ok {
		return
	}
	go func() {
		bg := context.WithoutCancel(ctx)
		defer l.unlock(bg, l.key+refreshLockSuffix, unlock)
		if _, err := l.loadAndStore(bg, true); err != nil {
			l.c.onError(bg, fmt.Errorf("gashe: refresh %q: %w", l.key, err))
		}
	}()
}

// loadLocked loads a missing key under the load lock and reads the result of whoever held the
// lock before. If the backend cannot lock, it loads without the lock.
func (l loading[V]) loadLocked(ctx context.Context) (V, error) {
	unlock, err := l.c.Lock(ctx, l.key+loadLockSuffix, l.o.LockOnMiss)
	if err != nil {
		if ctx.Err() != nil {
			var zero V
			return zero, err
		}
		l.c.onError(ctx, fmt.Errorf("gashe: load lock %q: %w", l.key, err))
		return l.loadAndStore(ctx, true)
	}
	defer l.unlock(ctx, l.key+loadLockSuffix, unlock)
	// Whoever held the lock may have stored a result we can answer with.
	if entry, err := lookup[stamped[V]](ctx, l.c, l.key); err == nil &&
		time.Now().Before(entry.At.Add(l.o.RefreshWindow)) {
		return entry.V, nil
	}
	return l.loadAndStore(ctx, true)
}

func (l loading[V]) unlock(ctx context.Context, key string, unlock UnlockFunc) {
	if err := unlock(ctx); err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: unlock %q: %w", key, err))
	}
}
