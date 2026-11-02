package gashe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// timedLoad runs load and measures how long it took.
func timedLoad[V any](ctx context.Context, load func(context.Context) (V, error)) (V, time.Duration, error) {
	start := time.Now()
	v, err := load(ctx)
	return v, time.Since(start), err
}

// stamped is a stored value with the moment it stops being fresh.
type stamped[V any] struct {
	At time.Time `json:"at"`
	V  V         `json:"v"`
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

// prefixed adds prefix to key as "prefix:key"; an empty prefix leaves key as it is.
func prefixed(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + ":" + key
}

// NotNil is a CacheIf rule that skips nil pointers, slices, maps, channels, functions and interfaces.
// Values of other kinds always pass.
func NotNil[V any](v V) bool {
	rv := reflect.ValueOf(any(v))
	if !rv.IsValid() {
		return false
	}
	switch rv.Kind() { //nolint:exhaustive // only nillable kinds matter
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		return !rv.IsNil()
	default:
		return true
	}
}
