package gashe

import (
	"cmp"
	"context"
	"fmt"
	"sync"
	"time"
)

// Options configures a Cache. The zero value is ready to use.
type Options struct {
	// Prefix is put in front of every key the cache stores, its own keys included, so several
	// services can share one Redis: "orders:" stores "orders:user:42".
	// Keys and All yield keys without it, and DeleteMatch never reaches past it. "" means none.
	// The cache keeps its own keys, tag counters, the locks of Stampede.LockFor and
	// Stale.WhileRevalidate and rate counters, under Prefix too, each starting with "__": keys of
	// yours must not start with "__". Locks of Lock, TryLock and WithLock use the key you give them.
	Prefix string
	// Hooks receive what the cache does: how calls were answered, and the errors it works around.
	// The zero value calls nothing.
	Hooks Hooks
	// CircuitBreaker stops calling the backend for a while once too many calls to it fail, as
	// Cache.CircuitBreaker does for a dependency. While it is open every call of the backend returns
	// ErrCircuitOpen at once, so a cache that is down costs no timeout: GetOrLoad goes straight to
	// the loader. The breaker lives in the memory of this process; misses are not failures, see
	// CircuitBreaker.IsFailure. Ping is not guarded, so health checks see the backend itself. The
	// zero value, with no FailurePercent, means no breaker.
	CircuitBreaker CircuitBreaker
	// Codec turns values into bytes: the format of what is stored. nil means JSON.
	Codec Codec
	// Compressor shrinks what Codec produced, such as compress/zstd for large values. nil means
	// none. Compressed values are binary: IncrBy cannot read them, so keep counters under keys
	// that only IncrBy touches.
	Compressor Compressor
	// LockPollInterval is how often Lock and Stampede.LockFor check whether a busy lock is free.
	// 0 means 100ms.
	LockPollInterval time.Duration
}

// Hooks are called by the cache, for metrics and logs. A nil hook is not called.
type Hooks struct {
	// OnResult receives how every GetOrLoad call, and every key of GetOrLoadMany, was answered:
	// use it to count hits and misses and to time the loader.
	// nil drops them.
	OnResult func(ctx context.Context, o Outcome)
	// OnError receives errors the cache works around instead of returning them, such as a
	// failing backend during GetOrLoad; each error names the operation and key.
	// nil drops them.
	OnError func(ctx context.Context, err error)
}

// Cache turns values into bytes, stores them in a Backend and runs caching, locks, rate limits
// and circuit breakers on top of it. It is safe for concurrent use.
type Cache struct {
	backend  Backend
	codec    Codec
	onError  func(ctx context.Context, err error)
	onResult func(ctx context.Context, o Outcome)
	lockPoll time.Duration
	calls    group
	breakers breakers
	loads    loadLimits
	shapes   sync.Map // reflect.Type to the uint64 of shapeOf
	bg       background
}

// New returns a Cache over b. It panics if b is nil, LockPollInterval is negative or CircuitBreaker
// is invalid.
//
//nolint:gocritic // options by value: they are built once, at startup
func New(b Backend, o Options) *Cache {
	if b == nil {
		panic("gashe: nil backend")
	}
	if o.LockPollInterval < 0 {
		panic("gashe: negative LockPollInterval")
	}
	if o.CircuitBreaker.FailurePercent != 0 {
		b = withBreaker(b, o.CircuitBreaker)
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
	if o.Hooks.OnError == nil {
		o.Hooks.OnError = func(context.Context, error) {}
	}
	if o.Hooks.OnResult == nil {
		o.Hooks.OnResult = func(context.Context, Outcome) {}
	}
	return &Cache{
		backend:  b,
		codec:    compressed{codec: o.Codec, compressor: o.Compressor},
		onError:  o.Hooks.OnError,
		onResult: o.Hooks.OnResult,
		lockPoll: cmp.Or(o.LockPollInterval, defaultLockPoll),
	}
}

// Backend returns the backend the cache stores bytes in, for raw access; it applies Options.Prefix
// and Options.CircuitBreaker.
func (c *Cache) Backend() Backend { return c.backend }

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
