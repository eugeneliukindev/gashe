package gashe

import (
	"context"
	"fmt"
	"time"
)

// Cached is one kind of data in a Cache: a Policy bound to it, checked once. Its methods are
// GetOrLoad, GetOrLoadMany and Reload with that policy, plus Set and Delete that apply
// Policy.Prefix and store in the format GetOrLoad reads. Keep one per kind of data, such as a
// field of a service, and share it; it is safe for concurrent use.
type Cached[V any] struct {
	c   *Cache
	o   Policy[V]
	ttl func(V) time.Duration
}

// For binds o to c. It panics on invalid options, as GetOrLoad would return ErrInvalidOptions on
// every call.
func For[V any](c *Cache, o Policy[V]) Cached[V] {
	ttl, err := o.check()
	if err != nil {
		panic(err)
	}
	return Cached[V]{c: c, o: o, ttl: ttl}
}

// GetOrLoad is GetOrLoad with the bound policy.
func (p Cached[V]) GetOrLoad(ctx context.Context, key string, load func(context.Context) (V, error)) (V, error) {
	return getOrLoad(ctx, p.c, key, p.o, p.ttl, load)
}

// GetOrLoadMany is GetOrLoadMany with the bound policy.
func (p Cached[V]) GetOrLoadMany(
	ctx context.Context,
	keys []string,
	load func(ctx context.Context, missing []string) (map[string]V, error),
) (map[string]V, error) {
	return getOrLoadMany(ctx, p.c, keys, p.o, p.ttl, load)
}

// Reload is Reload with the bound policy.
func (p Cached[V]) Reload(ctx context.Context, key string, load func(context.Context) (V, error)) (V, error) {
	return reload(ctx, p.c, key, p.o, p.ttl, load)
}

// Set stores v under key as GetOrLoad would store a loaded result, with the TTL, tags and shape
// of the policy, for example right after writing v to the database. A v that the policy does not
// store, by Store.If or a negative TTL, deletes the old entry instead.
func (p Cached[V]) Set(ctx context.Context, key string, v V) error {
	l := newLoading(p.c, p.o, p.ttl, key, nil)
	gens, err := l.readGens(ctx)
	if err != nil {
		return err
	}
	fresh, ok := l.freshFor(ctx, v)
	if !ok || !p.o.Store.accepts(v) {
		_, err = p.c.backend.Delete(ctx, l.key)
		return err
	}
	ttl := fresh + p.o.keep()
	raw, err := p.c.encode(l.key, entry[V]{At: time.Now().Add(fresh), V: v, G: gens, T: l.shape}, ttl)
	if err != nil {
		return err
	}
	if err := p.c.backend.Set(ctx, l.key, raw, ttl); err != nil {
		return fmt.Errorf("gashe: set %q: %w", l.key, err)
	}
	return nil
}

// Delete removes the entry under key, with Policy.Prefix, and reports whether it existed. A read
// that loaded before the delete may still store its older result; tags do not have this race.
func (p Cached[V]) Delete(ctx context.Context, key string) (existed bool, err error) {
	return p.c.backend.Delete(ctx, p.o.Prefix+key)
}
