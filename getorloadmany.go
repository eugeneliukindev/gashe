package gashe

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"time"
)

// GetOrLoadMany is GetOrLoad for many keys at once, for lists and pages that would otherwise make
// one query per key. It reads every key in one backend call and calls load once with the keys that
// were missing, so the loader can fetch them in one query; what load returns is stored in one call.
// The result holds every key that was found or loaded; keys load leaves out are absent and not
// stored.
//
// It reads entries stored by GetOrLoad with the same policy and the other way round. It supports
// TTL, TTLFunc, Jitter, Tags, CacheIf and Prefix; StaleWhileRevalidate, StaleIfError, LockOnMiss
// and Negative return ErrInvalidOptions. Unlike GetOrLoad, concurrent calls do not share a load.
// An error from load is returned as is; if the backend fails, the result of load is returned
// without storing it.
func GetOrLoadMany[V any](
	ctx context.Context,
	c *Cache,
	keys []string,
	o Policy[V],
	load func(ctx context.Context, missing []string) (map[string]V, error),
) (map[string]V, error) {
	if err := o.check(); err != nil {
		return nil, err
	}
	if o.keep() > 0 || o.LockOnMiss > 0 || o.Negative.If != nil {
		return nil, fmt.Errorf("%w: GetOrLoadMany does not support StaleWhileRevalidate, StaleIfError, "+
			"LockOnMiss or Negative", ErrInvalidOptions)
	}
	keys = slices.Compact(slices.Sorted(slices.Values(keys)))
	if len(keys) == 0 {
		return map[string]V{}, nil
	}
	l := loading[V]{c: c, o: o, shape: c.shapeOf(reflect.TypeFor[V]())}
	found, missing, err := l.readMany(ctx, keys)
	if err != nil {
		return load(ctx, keys)
	}
	if len(missing) == 0 {
		return found, nil
	}
	start := time.Now()
	loaded, err := load(ctx, missing)
	took := time.Since(start)
	for _, k := range missing {
		c.onResult(ctx, Outcome{Key: o.Prefix + k, Result: ResultMiss, Load: took})
	}
	if err != nil {
		return nil, err
	}
	maps.Copy(found, l.storeMany(ctx, missing, loaded))
	return found, nil
}

// readMany returns the fresh results under keys and the keys that have none. An error
// is a backend failure and is already reported.
func (l *loading[V]) readMany(
	ctx context.Context,
	keys []string,
) (found map[string]V, missing []string, err error) {
	full := make([]string, len(keys), len(keys)+len(l.o.Tags))
	for i, k := range keys {
		full[i] = l.o.Prefix + k
	}
	tagKeys := l.tagKeys()
	raws, err := l.c.backend.GetMany(ctx, append(full, tagKeys...)...)
	if err == nil {
		l.gens, err = l.c.generations(ctx, tagKeys, raws[len(keys):])
	}
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: get many: %w", err))
		return nil, nil, err
	}
	found = make(map[string]V, len(keys))
	now := time.Now()
	for i, k := range keys {
		var e entry[V]
		if raws[i] != nil {
			if err = l.c.decode(full[i], raws[i], &e); err != nil {
				l.c.onError(ctx, err)
			} else if l.usable(&e, now) {
				found[k] = e.V
				l.c.onResult(ctx, Outcome{Key: full[i], Result: ResultHit})
				continue
			}
		}
		missing = append(missing, k)
	}
	return found, missing, nil
}

// storeMany stores the loaded results of missing in one call and returns them as they read back.
// Every entry gets its own jittered fresh time; the backend keeps all of them as long as the
// longest, which costs a little memory and changes no answer.
func (l *loading[V]) storeMany(ctx context.Context, missing []string, loaded map[string]V) map[string]V {
	out := make(map[string]V, len(loaded))
	raws := make(map[string][]byte, len(loaded))
	var ttl time.Duration
	for _, k := range missing {
		v, ok := loaded[k]
		if !ok {
			continue
		}
		out[k] = v
		fresh, ok := l.freshFor(v)
		if !ok || (l.o.CacheIf != nil && !l.o.CacheIf(v)) {
			continue
		}
		key := l.o.Prefix + k
		raw, err := l.c.encode(key, entry[V]{At: time.Now().Add(fresh), V: v, G: l.gens, T: l.shape}, fresh)
		var back entry[V]
		if err == nil {
			err = l.c.decode(key, raw, &back)
		}
		if err != nil {
			l.c.onError(ctx, err)
			continue
		}
		out[k] = back.V
		raws[key] = raw
		ttl = max(ttl, fresh)
	}
	if len(raws) > 0 {
		if err := l.c.backend.SetMany(ctx, raws, ttl); err != nil {
			l.c.onError(ctx, fmt.Errorf("gashe: set many: %w", err))
		}
	}
	return out
}

// usable reports whether e answers without loading: fresh, of the current tag generations and of
// the current shape of V.
func (l *loading[V]) usable(e *entry[V], now time.Time) bool {
	return !e.Neg && now.Before(e.At) && slices.Equal(e.G, l.gens) && e.sameShape(l.shape)
}
