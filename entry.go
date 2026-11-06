package gashe

import (
	"context"
	"errors"
	"fmt"
	"time"
)

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
func store[V any](ctx context.Context, c *Cache, keep spare, key string, v V, ttl time.Duration) V {
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
		keep.put(ctx, key, raw, ttl) // kept in Policy.Fallback while the backend fails
	}
	return out
}
