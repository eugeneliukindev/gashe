package gashe

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"
	"time"
)

const (
	refreshLockSuffix = ":refresh"
	loadLockSuffix    = ":load"
	tagPrefix         = "gashe:tag:"
	// maxRefreshLock bounds the refresh lock and the background reload that holds it, so a
	// reload that hangs or a refresher that dies blocks refreshes for at most this long.
	maxRefreshLock = time.Minute
)

// Policy says how GetOrLoad caches results of type V. Define it once and pass it to
// every call; the zero value of each field means "off".
//
// A stored result is fresh for TTL: it is answered without calling the loader. After TTL it is
// old: StaleWhileRevalidate and StaleIfError say for how much longer it is still good enough. The
// entry is kept for TTL plus the longer of the two windows.
type Policy[V any] struct {
	// TTL is how long a result is fresh. It may be 0 when a window is set: then every call
	// either refreshes in the background (StaleWhileRevalidate) or calls the loader (StaleIfError).
	TTL time.Duration
	// TTLFunc, when set, picks TTL from the result. A negative TTL means "do not store".
	TTLFunc func(V) time.Duration
	// Jitter spreads TTLs randomly by up to this share, from 0 to 1 (0.1 means +-10%), so that
	// entries stored together do not expire together.
	Jitter float64
	// StaleWhileRevalidate is how long after TTL an old result is still answered at once, while one
	// caller reloads it in the background. The refresh lock is shared by every process that uses
	// the same backend.
	StaleWhileRevalidate time.Duration
	// StaleIfError is how long after TTL an old result answers a loader error, so callers keep
	// getting data while the source is down.
	StaleIfError time.Duration
	// StaleIf decides which loader errors StaleIfError covers. nil means any error.
	StaleIf func(error) bool
	// LockOnMiss, when set, makes a missing key load under a lock: one process calls the loader,
	// the others wait and take its result. The value is the longest the lock is held. The lock is
	// shared by every process that uses the same backend.
	LockOnMiss time.Duration
	// Negative caches answers like "not found", so lookups of missing data do not reach the
	// source every time.
	Negative Negative
	// Tags lists the tags of a result; Cache.InvalidateTags of any of them makes it a miss. Tags
	// are shared by every process that uses the same backend.
	Tags []string
	// CacheIf decides whether a result is stored. nil stores every successful result.
	CacheIf func(V) bool
	// Prefix is put in front of every key as is, separator included: "v2:" stores "v2:user:42".
	Prefix string
}

// Negative caches loader errors that mean "there is no such data".
type Negative struct {
	// If picks the loader errors to cache. nil turns negative caching off.
	If func(error) bool
	// Err is the error GetOrLoad returns while the negative answer is cached. Return a sentinel
	// your code already checks, such as sql.ErrNoRows.
	Err error
	// TTL is how long the negative answer is kept.
	TTL time.Duration
}

// Result says how a GetOrLoad call was answered; Options.OnResult receives it.
type Result int

const (
	// ResultHit is a fresh stored result.
	ResultHit Result = iota + 1
	// ResultMiss means the loader ran.
	ResultMiss
	// ResultStaleWhileRevalidate is a stale stored result, answered while it is reloaded in the
	// background.
	ResultStaleWhileRevalidate
	// ResultStaleIfError is a stale stored result, answered instead of a loader error.
	ResultStaleIfError
	// ResultNegativeHit is a cached negative answer.
	ResultNegativeHit
)

// String returns the name of r.
func (r Result) String() string {
	switch r {
	case ResultHit:
		return "hit"
	case ResultMiss:
		return "miss"
	case ResultStaleWhileRevalidate:
		return "stale-while-revalidate"
	case ResultStaleIfError:
		return "stale-if-error"
	case ResultNegativeHit:
		return "negative-hit"
	}
	return "Result(" + strconv.Itoa(int(r)) + ")"
}

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

// InvalidateTags makes every result stored with one of tags a miss. Every tag has a generation
// counter that a result remembers when it is stored; this bumps the counters, so it costs one
// write per tag and no scan of the backend. Tags are shared by every process that uses the same
// backend.
func (c *Cache) InvalidateTags(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		if _, err := c.backend.Incr(ctx, tagPrefix+tag, 1, 0); err != nil {
			return err
		}
	}
	return nil
}

func (o Policy[V]) check() error {
	for _, rule := range []struct {
		broken bool
		why    string
	}{
		{
			negative(o.TTL, o.StaleWhileRevalidate, o.StaleIfError, o.LockOnMiss, o.Negative.TTL),
			"durations must not be negative",
		},
		{o.Jitter < 0 || o.Jitter >= 1, "Jitter must be from 0 to less than 1"},
		{o.TTL == 0 && o.TTLFunc == nil && o.keep() == 0, "set TTL, TTLFunc or a window, or nothing is ever stored"},
		{o.Negative.If != nil && (o.Negative.Err == nil || o.Negative.TTL == 0), "Negative needs Err and TTL"},
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
func (o Policy[V]) keep() time.Duration { return max(o.StaleWhileRevalidate, o.StaleIfError) }

// spread applies Jitter to d.
func (o Policy[V]) spread(d time.Duration) time.Duration {
	if o.Jitter == 0 || d <= 0 {
		return d
	}
	return time.Duration(float64(d) * (1 + (rand.Float64()*2-1)*o.Jitter)) //nolint:gosec // jitter, not a secret
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

func (l *loading[V]) readGens(ctx context.Context) ([]int64, error) {
	if len(l.o.Tags) == 0 {
		return nil, nil
	}
	raws, err := l.c.backend.GetMany(ctx, l.tagKeys()...)
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: get tags of %q: %w", l.key, err))
		return nil, err
	}
	return generations(raws), nil
}

func (l *loading[V]) tagKeys() []string {
	keys := make([]string, len(l.o.Tags))
	for i, g := range l.o.Tags {
		keys[i] = tagPrefix + g
	}
	return keys
}

// generations parses tag counters; a missing counter is generation 0.
func generations(raws [][]byte) []int64 {
	gens := make([]int64, len(raws))
	for i, raw := range raws {
		gens[i], _ = strconv.ParseInt(string(raw), 10, 64)
	}
	return gens
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
