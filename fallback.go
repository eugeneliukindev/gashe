package gashe

import (
	"cmp"
	"context"
	"fmt"
	"time"
)

// defaultFallbackTTL is the MaxTTL of a fallback that sets none.
const defaultFallbackTTL = 30 * time.Second

// Fallback configures Policy.Fallback: where loads keep their results while the backend of
// the cache fails, so the source gets one load per key and process, not one per call.
type Fallback struct {
	// Backend keeps the results, usually a memory.Backend of its own; without it there is no
	// fallback. Policies of one
	// Cache may share it; a near cache or another Cache may not.
	// nil, the zero value, means none: while the backend fails, every call loads.
	Backend Backend
	// MaxTTL is the longest a result stays in Backend: the TTL of its policy, but never longer, since
	// no invalidation reaches Backend while the backend is down. 0 means 30 seconds.
	MaxTTL time.Duration
}

// spare keeps results in Policy.Fallback.Backend while the backend fails; without a Fallback it
// keeps nothing.
type spare struct {
	backend Backend // nil without a Fallback
	maxTTL  time.Duration
	onError func(ctx context.Context, err error)
}

// spareOf returns the spare of a policy with fallback f, already checked by Policy.check.
func (c *Cache) spareOf(f Fallback) spare {
	if f.Backend == nil {
		return spare{}
	}
	return spare{backend: f.Backend, maxTTL: cmp.Or(f.MaxTTL, defaultFallbackTTL), onError: c.onError}
}

func (s spare) get(ctx context.Context, key string) ([]byte, bool) {
	if s.backend == nil {
		return nil, false
	}
	raw, err := s.backend.Get(ctx, key)
	return raw, err == nil
}

// put keeps raw for ttl, but never longer than MaxTTL; a result never fresh keeps MaxTTL.
func (s spare) put(ctx context.Context, key string, raw []byte, ttl time.Duration) {
	if s.backend == nil {
		return
	}
	if ttl <= 0 || ttl > s.maxTTL {
		ttl = s.maxTTL
	}
	if err := s.backend.Set(ctx, key, raw, ttl); err != nil {
		s.onError(ctx, fmt.Errorf("gashe: fallback set %q: %w", key, err))
	}
}

// withoutBackend answers while the backend fails: from Policy.Fallback if it has the key, a result
// or a cached "not found", or by loading and keeping the answer there.
func (l *loading[V]) withoutBackend(ctx context.Context) (V, Result, error) {
	if e, ok := l.sparedEntry(ctx, l.key); ok {
		if e.Neg {
			var zero V
			return zero, ResultNegativeHit, ErrNotFound
		}
		return e.V, ResultFallback, nil
	}
	v, err := l.call(ctx)
	switch {
	case err == nil:
		l.spareResult(ctx, l.key, v)
	case l.notFound(err):
		l.spareNegative(ctx)
		err = notFoundError(err)
	}
	return v, ResultMiss, err
}

// spared returns the result Policy.Fallback keeps under key, if it has one.
func (l *loading[V]) spared(ctx context.Context, key string) (V, bool) {
	e, ok := l.sparedEntry(ctx, key)
	return e.V, ok && !e.Neg
}

// sparedEntry returns the entry Policy.Fallback keeps under key, if it is of the current shape of V.
func (l *loading[V]) sparedEntry(ctx context.Context, key string) (entry[V], bool) {
	var e entry[V]
	raw, ok := l.spare.get(ctx, key)
	if !ok || l.c.decode(key, raw, &e) != nil || !e.sameShape(l.shape) {
		return e, false
	}
	return e, true
}

// spareNegative keeps a "not found" in Policy.Fallback for Negative.TTL.
func (l *loading[V]) spareNegative(ctx context.Context) {
	ttl := l.o.spread(l.o.Negative.TTL)
	raw, err := l.c.encode(l.key, entry[V]{At: time.Now().Add(ttl), Neg: true, T: l.shape}, 0)
	if err != nil {
		l.c.onError(ctx, err)
		return
	}
	l.spare.put(ctx, l.key, raw, ttl)
}

// spareResult keeps v in Policy.Fallback for its TTL, if the policy stores it.
func (l *loading[V]) spareResult(ctx context.Context, key string, v V) {
	if !l.o.Store.stores(v, l.took) {
		return
	}
	fresh, ok := l.freshFor(ctx, v)
	if !ok {
		return
	}
	raw, err := l.c.encode(key, entry[V]{At: time.Now().Add(fresh), V: v, T: l.shape}, 0)
	if err != nil {
		l.c.onError(ctx, err)
		return
	}
	l.spare.put(ctx, key, raw, fresh)
}

// manyWithoutBackend answers keys while the backend fails: from Policy.Fallback for the keys it
// has, and one load for the others, whose results it keeps there.
func (l *loading[V]) manyWithoutBackend(
	ctx context.Context,
	keys []string,
	load func(ctx context.Context, missing []string) (map[string]V, error),
) (map[string]V, error) {
	found := make(map[string]V, len(keys))
	var missing []string
	for _, k := range keys {
		if v, ok := l.spared(ctx, l.o.Prefix+k); ok {
			found[k] = v
			l.c.onResult(ctx, Outcome{Key: l.o.Prefix + k, Result: ResultFallback})
			continue
		}
		missing = append(missing, k)
	}
	if len(missing) == 0 {
		return found, nil
	}
	loaded, err := l.loadMany(ctx, load, missing)
	if err != nil {
		return nil, err
	}
	for k, v := range loaded {
		found[k] = v
		l.spareResult(ctx, l.o.Prefix+k, v)
	}
	return found, nil
}
