package gashe

import (
	"cmp"
	"context"
	"fmt"
	"iter"
	"sync"
	"time"
)

// allBatch is how many keys All reads from the backend at once.
const allBatch = 100

// Options configures a Cache. The zero value is ready to use.
type Options struct {
	// Prefix is put in front of every key the cache stores, its own keys included, so several
	// services can share one Redis: "orders:" stores "orders:user:42".
	// Keys and All yield keys without it, and DeleteMatch never reaches past it. "" means none.
	// The cache keeps its own keys, tag counters, locks and rate counters, under Prefix too, each
	// starting with "_": keys of yours must not start with "_".
	Prefix string
	// Codec turns values into bytes: the format of what is stored. nil means JSON.
	Codec Codec
	// Compressor shrinks what Codec produced, such as compress/zstd for large values. nil means
	// none. Compressed values are binary: IncrBy cannot read them, so keep counters under keys
	// that only IncrBy touches.
	Compressor Compressor
	// CircuitBreaker stops calling the backend for a while once too many calls to it fail, as
	// Cache.CircuitBreaker does for a dependency. While it is open every call of the backend returns
	// ErrCircuitOpen at once, so a cache that is down costs no timeout: GetOrLoad goes straight to
	// the loader. The breaker lives in the memory of this process; misses are not failures, see
	// CircuitBreakerOptions.IsFailure. Ping is not guarded, so health checks see the backend
	// itself. nil means no breaker.
	CircuitBreaker *CircuitBreakerOptions
	// Fallback keeps the results of GetOrLoad, GetOrLoadMany and Reload in a backend of its own
	// while the backend fails or CircuitBreaker is open, for at most Fallback.MaxTTL, so the source
	// gets one load per key and process instead of one per call. The basic API and locks do not
	// use it: they return the error. nil means none: while the backend fails, every call loads.
	Fallback *FallbackOptions
	// OnError receives errors the cache works around instead of returning them, such as a
	// failing backend during GetOrLoad; each error names the operation and key. nil drops them.
	OnError func(ctx context.Context, err error)
	// OnResult receives how every GetOrLoad call, and every key of GetOrLoadMany, was answered:
	// use it to count hits and misses and to time the loader. nil drops them.
	OnResult func(ctx context.Context, o Outcome)
	// LockPollInterval is how often Lock and LockOnMiss check whether a busy lock is free.
	// 0 means 100ms.
	LockPollInterval time.Duration
}

// Cache turns values into bytes, stores them in a Backend and runs caching, locks, rate limits
// and circuit breakers on top of it. It is safe for concurrent use.
type Cache struct {
	backend  Backend
	codec    Codec
	onError  func(ctx context.Context, err error)
	onResult func(ctx context.Context, o Outcome)
	spare    spare // Options.Fallback
	lockPoll time.Duration
	calls    group
	breakers breakers
	shapes   sync.Map // reflect.Type to the uint64 of shapeOf
	bg       background
}

// New returns a Cache over b. It panics if b is nil, LockPollInterval is negative, or CircuitBreaker
// or Fallback is invalid.
//
//nolint:gocritic // options by value: they are built once, at startup
func New(b Backend, o Options) *Cache {
	if b == nil {
		panic("gashe: nil backend")
	}
	if o.LockPollInterval < 0 {
		panic("gashe: negative LockPollInterval")
	}
	if o.CircuitBreaker != nil {
		b = withBreaker(b, *o.CircuitBreaker)
	}
	if o.Prefix != "" {
		b = withPrefix(b, o.Prefix)
	}
	if o.Codec == nil {
		o.Codec = JSON{}
	}
	if o.Compressor == nil {
		o.Compressor = noCompression{}
	}
	if o.OnError == nil {
		o.OnError = func(context.Context, error) {}
	}
	if o.OnResult == nil {
		o.OnResult = func(context.Context, Outcome) {}
	}
	var keep spare = noSpare{}
	if o.Fallback != nil {
		keep = newSpare(o.Fallback, o.OnError)
	}
	return &Cache{
		backend:  b,
		codec:    compressed{codec: o.Codec, compressor: o.Compressor},
		onError:  o.OnError,
		onResult: o.OnResult,
		spare:    keep,
		lockPoll: cmp.Or(o.LockPollInterval, defaultLockPoll),
	}
}

// Backend returns the backend the cache stores bytes in, for raw access; it applies Options.Prefix
// and Options.CircuitBreaker.
func (c *Cache) Backend() Backend { return c.backend }

// Get decodes the value stored under key into dst. A miss returns ErrNotFound.
func (c *Cache) Get(ctx context.Context, key string, dst any) error {
	raw, err := c.backend.Get(ctx, key)
	if err != nil {
		return err
	}
	return c.decode(key, raw, dst)
}

// Set stores v under key. ttl 0 means no expiry; an existing entry and its TTL are replaced.
func (c *Cache) Set(ctx context.Context, key string, v any, ttl time.Duration) error {
	raw, err := c.encode(key, v, ttl)
	if err != nil {
		return err
	}
	return c.backend.Set(ctx, key, raw, ttl)
}

// SetIfAbsent stores v only if key has no live entry and reports whether it did.
func (c *Cache) SetIfAbsent(ctx context.Context, key string, v any, ttl time.Duration) (stored bool, err error) {
	raw, err := c.encode(key, v, ttl)
	if err != nil {
		return false, err
	}
	return c.backend.SetIfAbsent(ctx, key, raw, ttl)
}

// SetIfPresent stores v only if key has a live entry and reports whether it did.
func (c *Cache) SetIfPresent(ctx context.Context, key string, v any, ttl time.Duration) (stored bool, err error) {
	raw, err := c.encode(key, v, ttl)
	if err != nil {
		return false, err
	}
	return c.backend.SetIfPresent(ctx, key, raw, ttl)
}

// GetMany returns one Value per key, in order. Missing keys give values that are not found.
func (c *Cache) GetMany(ctx context.Context, keys ...string) ([]Value, error) {
	raws, err := c.backend.GetMany(ctx, keys...)
	if err != nil {
		return nil, err
	}
	values := make([]Value, len(keys))
	for i, raw := range raws {
		values[i] = Value{key: keys[i], raw: raw, codec: c.codec}
	}
	return values, nil
}

// SetMany stores all values with the same TTL.
func (c *Cache) SetMany(ctx context.Context, values map[string]any, ttl time.Duration) error {
	if ttl < 0 {
		return ErrInvalidTTL
	}
	raws := make(map[string][]byte, len(values))
	for key, v := range values {
		raw, err := c.encode(key, v, ttl)
		if err != nil {
			return err
		}
		raws[key] = raw
	}
	return c.backend.SetMany(ctx, raws, ttl)
}

// Keys yields every key that matches pattern. A failure is yielded once as the error.
func (c *Cache) Keys(ctx context.Context, pattern string) iter.Seq2[string, error] {
	return c.backend.Scan(ctx, pattern)
}

// All yields every live entry whose key matches pattern, reading values in batches while the
// scan goes on. A failure is yielded once as the error.
func (c *Cache) All(ctx context.Context, pattern string) iter.Seq2[Value, error] {
	return func(yield func(Value, error) bool) {
		batch := make([]string, 0, allBatch)
		// flush yields the batch and reports whether to go on.
		flush := func() bool {
			values, err := c.GetMany(ctx, batch...)
			batch = batch[:0]
			if err != nil {
				yield(Value{}, err)
				return false
			}
			for _, v := range values {
				// A key can expire between Scan and GetMany.
				if v.Found() && !yield(v, nil) {
					return false
				}
			}
			return true
		}
		for key, err := range c.backend.Scan(ctx, pattern) {
			if err != nil {
				yield(Value{}, err)
				return
			}
			if batch = append(batch, key); len(batch) == allBatch && !flush() {
				return
			}
		}
		if len(batch) > 0 {
			flush()
		}
	}
}

// Delete removes key and reports whether it existed.
func (c *Cache) Delete(ctx context.Context, key string) (existed bool, err error) {
	return c.backend.Delete(ctx, key)
}

// DeleteMany removes all keys.
func (c *Cache) DeleteMany(ctx context.Context, keys ...string) error {
	return c.backend.DeleteMany(ctx, keys...)
}

// DeleteMatch removes every key that matches pattern.
func (c *Cache) DeleteMatch(ctx context.Context, pattern string) error {
	return c.backend.DeleteMatch(ctx, pattern)
}

// Exists reports whether key has a live entry.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	return c.backend.Exists(ctx, key)
}

// IncrBy adds by to the integer under key, creating it from 0 if absent, and returns the result.
// ttl applies only when the entry is created. The stored integer reads back with Get.
func (c *Cache) IncrBy(ctx context.Context, key string, by int64, ttl time.Duration) (value int64, err error) {
	if ttl < 0 {
		return 0, ErrInvalidTTL
	}
	return c.backend.IncrBy(ctx, key, by, ttl)
}

// Expire sets a new TTL for key and reports whether the key existed. ttl must be positive.
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) (existed bool, err error) {
	if ttl <= 0 {
		return false, ErrInvalidTTL
	}
	return c.backend.Expire(ctx, key, ttl)
}

// TTL returns the time key has left, or NoExpiry. A miss returns ErrNotFound.
func (c *Cache) TTL(ctx context.Context, key string) (left time.Duration, err error) {
	return c.backend.TTL(ctx, key)
}

// Ping checks that the backend is reachable.
func (c *Cache) Ping(ctx context.Context) error { return c.backend.Ping(ctx) }

func (c *Cache) encode(key string, v any, ttl time.Duration) ([]byte, error) {
	if ttl < 0 {
		return nil, ErrInvalidTTL
	}
	raw, err := c.codec.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("gashe: encode %q: %w", key, err)
	}
	return raw, nil
}

func (c *Cache) decode(key string, raw []byte, dst any) error {
	if err := c.codec.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("gashe: decode %q: %w", key, err)
	}
	return nil
}

// Value is one result of GetMany or All.
type Value struct {
	key   string
	raw   []byte
	codec Codec
}

// Key returns the key the value was read from.
func (v Value) Key() string { return v.key }

// Found reports whether the key had a live entry.
func (v Value) Found() bool { return v.raw != nil }

// Decode decodes the value into dst. A value that was not found returns ErrNotFound.
func (v Value) Decode(dst any) error {
	if v.raw == nil {
		return ErrNotFound
	}
	if err := v.codec.Unmarshal(v.raw, dst); err != nil {
		return fmt.Errorf("gashe: decode %q: %w", v.key, err)
	}
	return nil
}
