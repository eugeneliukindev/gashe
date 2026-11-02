# gashe API reference

Every exported function, method, type and field: what it takes and what it returns. When to use
which strategy is explained in the [README](README.md#strategies). The same text is in the doc
comments, so `go doc github.com/eugeneliukindev/gashe.GetOrLoad` shows it too.

- [Common parameters](#common-parameters)
- [Package gashe](#package-gashe)
  - [Creating a cache](#creating-a-cache)
  - [Caching](#caching)
  - [Invalidation](#invalidation)
  - [Locks](#locks)
  - [Rate limits](#rate-limits)
  - [Circuit breaker](#circuit-breaker)
  - [Basic API](#basic-api)
  - [Codecs](#codecs)
  - [Backends](#backends)
  - [Errors](#errors)
- [Package gashe/backend/memory](#package-gashebackendmemory)
- [Package gashe/backend/nearcache](#package-gashebackendnearcache)
- [Package gashe/backend/redis](#package-gashebackendredis)
- [Package gashe/codec/msgpack](#package-gashecodecmsgpack)
- [Package gashe/backendtest](#package-gashebackendtest)

## Common parameters

These parameters mean the same in every call.

| Parameter | Type | Meaning |
|---|---|---|
| `ctx` | `context.Context` | Cancels the call and carries its deadline. |
| `c` | `*gashe.Cache` | The cache to work with. |
| `key` | `string` | The key of the entry, without `Policy.Prefix` (see [Keys and TTL](README.md#keys-and-ttl)). |
| `keys` | `[]string` or `...string` | Several keys. |
| `ttl` | `time.Duration` | How long the entry lives. In the basic API 0 means "never expires" and a negative value returns `ErrInvalidTTL`; locks and `Expire` need a positive one. |
| `pattern` | `string` | A Redis glob: `*`, `?`, `[abc]`, `[^a]`, `[a-z]`, `\` to escape. |
| `o` | `Policy[V]`, `BreakerOptions` | The options of the call. Invalid ones return `ErrInvalidOptions`. |
| `load` | a function | Your loader: it is called when the cache cannot answer. |

## Package gashe

`import "github.com/eugeneliukindev/gashe"`

### Creating a cache

#### New

```go
func New(b Backend, o Options) *Cache
```

Returns a cache over the backend `b`. Create one at startup and share it; it is safe for
concurrent use. It does not close `b`. Panics if `b` is nil or `o.LockPollInterval` is negative.

#### Options

```go
type Options struct {
	Codec            Codec
	OnError          func(ctx context.Context, err error)
	OnResult         func(ctx context.Context, key string, r Result)
	LockPollInterval time.Duration
}
```

| Field | Zero value | Meaning |
|---|---|---|
| `Codec` | `JSON{}` | Turns values into bytes and back. |
| `OnError` | errors are dropped | Receives errors the cache works around: a failed backend in `GetOrLoad`, `RateLimit`, a background reload. |
| `OnResult` | not called | Receives how every `GetOrLoad` and every key of `GetOrLoadMany` was answered, with the stored key. |
| `LockPollInterval` | 100ms | How often `Lock` and `LockOnMiss` check a busy lock. |

#### Cache.Shutdown

```go
func (c *Cache) Shutdown(ctx context.Context) error
```

Stops starting background reloads and waits for the running ones. Returns `nil` when they are
done, or `ctx.Err()` if `ctx` ends first. The cache keeps working afterwards. Call it before closing
the backend.

### Caching

#### GetOrLoad

```go
func GetOrLoad[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	load func(context.Context) (V, error),
) (V, error)
```

Returns the stored result under `o.Prefix + key`, or calls `load`, stores its result and returns it.

- Takes: `V` is the type of the result; `o` says how it is cached; `load` returns the real value.
- Returns: the result and `nil`; or the error of `load` as is (not stored unless `StaleIfError`
  answers it or `Negative` caches it); `o.Negative.Err` while a negative answer is cached;
  `ErrInvalidOptions` for invalid `o`.
- If the backend fails, returns the result of `load` without storing it and reports the error to
  `OnError`.
- Concurrent calls with the same key and type in one process share one load.

#### GetOrLoadMany

```go
func GetOrLoadMany[V any](
	ctx context.Context,
	c *Cache,
	keys []string,
	o Policy[V],
	load func(ctx context.Context, missing []string) (map[string]V, error),
) (map[string]V, error)
```

`GetOrLoad` for many keys: one backend read, one call of `load` with the keys that were missing,
one backend write.

- Takes: `keys` may repeat and come in any order; `load` gets every missing key once, sorted, and
  returns the values it found, by key.
- Returns: a map with every key that was found or loaded; keys `load` leaves out are absent and not
  stored. On an error of `load` returns `nil` and that error.
- Supports `TTL`, `TTLFunc`, `Jitter`, `Tags`, `CacheIf`, `Prefix`. `StaleWhileRevalidate`,
  `StaleIfError`, `LockOnMiss` and `Negative` return `ErrInvalidOptions`.
- Reads and writes the same entries as `GetOrLoad` with the same policy. Concurrent calls are not
  shared.

#### Reload

```go
func Reload[V any](
	ctx context.Context,
	c *Cache,
	key string,
	o Policy[V],
	load func(context.Context) (V, error),
) (V, error)
```

Calls `load` and stores its result whatever is stored, for example after the data changed.
Returns the result, or the error of `load`, which leaves the stored result alone.

#### Policy

```go
type Policy[V any] struct {
	TTL                  time.Duration
	TTLFunc              func(V) time.Duration
	Jitter               float64
	StaleWhileRevalidate time.Duration
	StaleIfError         time.Duration
	StaleIf              func(error) bool
	LockOnMiss           time.Duration
	Negative             Negative
	Tags                 []string
	CacheIf              func(V) bool
	Prefix               string
}
```

How results of type `V` are cached. Define it once per kind of data. The zero value of every
field means "off". At least one of `TTL`, `TTLFunc`, `StaleWhileRevalidate`, `StaleIfError` must
be set. Durations must not be negative.

| Field | Meaning |
|---|---|
| `TTL` | How long a result is fresh: answered without calling the loader. |
| `TTLFunc` | Picks the TTL from the result instead of `TTL`. A negative TTL means "do not store". |
| `Jitter` | Spreads TTLs randomly by this share, from 0 to less than 1: `0.1` is +-10%. |
| `StaleWhileRevalidate` | How long after TTL a stale result is answered at once while one caller reloads it in the background. |
| `StaleIfError` | How long after TTL a stale result answers a loader error. |
| `StaleIf` | Which loader errors `StaleIfError` covers. nil means any. |
| `LockOnMiss` | On a missing key, one process loads under a lock held at most this long; the others wait for its result. |
| `Negative` | Caches "not found" errors, see below. |
| `Tags` | The tags of the result; `InvalidateTags` with any of them makes it a miss. |
| `CacheIf` | Decides whether a result is stored. nil stores every successful result. |
| `Prefix` | Put in front of every key as is, separator included: `"user:"` stores `"user:42"`. |

#### Negative

```go
type Negative struct {
	If  func(error) bool
	Err error
	TTL time.Duration
}
```

| Field | Meaning |
|---|---|
| `If` | Picks the loader errors to cache. nil turns negative caching off. |
| `Err` | The error `GetOrLoad` returns while the negative answer is cached. Required with `If`. |
| `TTL` | How long the negative answer is kept. Required with `If`. |

#### NotNil

```go
func NotNil[V any](v V) bool
```

A ready `CacheIf`: false for nil pointers, slices, maps, channels, functions and interfaces, true
for everything else.

#### Result

```go
type Result int

const (
	ResultHit                  // a fresh stored result
	ResultMiss                 // the loader ran
	ResultStaleWhileRevalidate // a stale result, reloaded in the background
	ResultStaleIfError         // a stale result instead of a loader error
	ResultNegativeHit          // a cached negative answer
)

func (r Result) String() string
```

What `Options.OnResult` receives. `String` returns `"hit"`, `"miss"`, `"stale-while-revalidate"`,
`"stale-if-error"` or `"negative-hit"`.

### Invalidation

```go
func (c *Cache) InvalidateTags(ctx context.Context, tags ...string) error
func (c *Cache) Delete(ctx context.Context, key string) (bool, error)
func (c *Cache) DeleteMany(ctx context.Context, keys ...string) error
func (c *Cache) DeleteMatch(ctx context.Context, pattern string) error
```

| Method | Takes | Returns |
|---|---|---|
| `InvalidateTags` | the tags to invalidate | an error of the backend. Every result stored with one of `tags` becomes a miss; one write per tag. |
| `Delete` | the key as stored, with `Policy.Prefix` | whether the key existed. |
| `DeleteMany` | keys as stored | an error of the backend. |
| `DeleteMatch` | a pattern | an error of the backend. Scans the whole backend. |

### Locks

```go
func (c *Cache) Lock(ctx context.Context, key string, ttl time.Duration) (*Lease, error)
func (c *Cache) TryLock(ctx context.Context, key string, ttl time.Duration) (*Lease, bool, error)
func (c *Cache) IsLocked(ctx context.Context, key string) (bool, error)
func (l *Lease) Extend(ctx context.Context, ttl time.Duration) error
func (l *Lease) Unlock(ctx context.Context) error
```

| Method | Takes | Returns |
|---|---|---|
| `Lock` | the lock key; `ttl > 0`, after which the lock frees itself if its holder dies | the lease once the lock is taken; waits while it is busy, checking every `LockPollInterval`; `ctx.Err()` if `ctx` ends first. |
| `TryLock` | the same | the lease and `true`, or `nil, false` if someone holds the lock; does not wait. |
| `IsLocked` | the lock key | whether someone holds it. |
| `Lease.Extend` | a new `ttl > 0`, counted from now | `ErrLockLost` if the lock expired and someone else may hold it. |
| `Lease.Unlock` | - | an error of the backend. Releases the lock only if it is still yours, even after `ctx` was cancelled. |

`Lease` has no exported fields; you get it from `Lock` or `TryLock`.

### Rate limits

```go
func (c *Cache) RateLimit(ctx context.Context, key string, limit Limit) error

type Limit interface{ /* SlidingWindow or FixedWindow */ }

type SlidingWindow struct {
	Calls int
	Per   time.Duration
}

func PerSecond(n int) SlidingWindow
func PerMinute(n int) SlidingWindow
func PerHour(n int) SlidingWindow

type FixedWindow struct {
	Calls  int
	Per    time.Duration
	BanFor time.Duration
}
```

`RateLimit` counts a call for `key` and returns `ErrRateLimited` once the key is over `limit`, or
`nil`. Call it before the work it guards. If the backend fails it reports to `OnError` and returns
`nil`. Invalid limits return `ErrInvalidOptions`.

| Type, field | Meaning |
|---|---|
| `SlidingWindow.Calls`, `.Per` | About `Calls` calls in any span of `Per` ending now. Both must be positive. Refused calls are not counted. |
| `PerSecond(n)`, `PerMinute(n)`, `PerHour(n)` | `SlidingWindow{Calls: n, Per: time.Second}` and so on. |
| `FixedWindow.Calls`, `.Per` | At most `Calls` calls per span of `Per` that starts with the first call. Both must be positive. |
| `FixedWindow.BanFor` | How long the key stays limited after it first goes over. 0 means `Per`. |

### Circuit breaker

```go
func (c *Cache) CircuitBreaker(key string, o BreakerOptions) (DoneFunc, error)

type DoneFunc func(err error)

type BreakerOptions struct {
	FailurePercent int
	MinCalls       int
	Window         time.Duration
	OpenFor        time.Duration
	HalfOpenCalls  int
	IsFailure      func(error) bool
}
```

`CircuitBreaker` checks the breaker of `key` before a call to a dependency. It returns
`ErrCircuitOpen` while the breaker is open, or a `DoneFunc`: call it once with the error of your
call, `nil` for success. It needs no `ctx`: the breaker lives in the memory of this process. Use the
same options for the same key.

| Field | Zero value | Meaning |
|---|---|---|
| `FailurePercent` | required | Opens the breaker once this share of calls in `Window` failed, from 1 to 99. |
| `MinCalls` | 1 | How many calls `Window` needs before the breaker may open. |
| `Window` | required | The span of recent time calls are counted over. |
| `OpenFor` | required | How long the open breaker refuses calls. |
| `HalfOpenCalls` | the breaker just closes after `OpenFor` | How many test calls are let through after `OpenFor`: the first success closes it, a failure opens it again. |
| `IsFailure` | any error | Which errors count as failures. |

### Basic API

Works like a map with TTL. Values are encoded with `Options.Codec`; keys are used as given.

```go
func (c *Cache) Get(ctx context.Context, key string, dst any) error
func (c *Cache) Set(ctx context.Context, key string, v any, ttl time.Duration) error
func (c *Cache) SetIfAbsent(ctx context.Context, key string, v any, ttl time.Duration) (bool, error)
func (c *Cache) SetIfPresent(ctx context.Context, key string, v any, ttl time.Duration) (bool, error)
func (c *Cache) GetMany(ctx context.Context, keys ...string) ([]Value, error)
func (c *Cache) SetMany(ctx context.Context, values map[string]any, ttl time.Duration) error
func (c *Cache) Keys(ctx context.Context, pattern string) iter.Seq2[string, error]
func (c *Cache) All(ctx context.Context, pattern string) iter.Seq2[Value, error]
func (c *Cache) Exists(ctx context.Context, key string) (bool, error)
func (c *Cache) Incr(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error)
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error)
func (c *Cache) Ping(ctx context.Context) error
func (c *Cache) Backend() Backend
```

| Method | Takes | Returns |
|---|---|---|
| `Get` | `dst`, a pointer to decode into | `ErrNotFound` on a miss. |
| `Set` | the value `v` | an error. Replaces the value and its TTL. |
| `SetIfAbsent` | the value `v` | whether it stored: only if the key had no live entry. |
| `SetIfPresent` | the value `v` | whether it stored: only if the key had a live entry. |
| `GetMany` | keys | one `Value` per key, in order; missing keys give values that are not found. |
| `SetMany` | values by key | an error. All get the same TTL. |
| `Keys` | a pattern | an iterator over matching keys; a failure is yielded once as the error. May yield a key twice on Redis. |
| `All` | a pattern | an iterator over matching live entries, read in batches of 100. |
| `Exists` | a key | whether it has a live entry. |
| `Incr` | `by`, which may be negative; `ttl`, used only when the counter is created | the new value. `ErrWrongType` if the value is not an integer. |
| `Expire` | `ttl > 0` | whether the key existed. |
| `TTL` | a key | the time left, `NoExpiry` for an entry without TTL, `ErrNotFound` on a miss. |
| `Ping` | - | an error if the backend is unreachable. |
| `Backend` | - | the backend, for raw access to bytes. |

#### Value

```go
func (v Value) Key() string
func (v Value) Found() bool
func (v Value) Decode(dst any) error
```

One result of `GetMany` or `All`. `Key` returns the key, `Found` whether it had a live entry,
`Decode` decodes it into `dst` or returns `ErrNotFound`.

### Codecs

```go
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

type JSON struct{}
type Gzip struct{ Inner Codec }
type Raw struct{ Inner Codec }
```

| Type | Meaning |
|---|---|
| `Codec` | Implement it to store values another way. |
| `JSON` | The default, `encoding/json`. |
| `Gzip` | Compresses what `Inner` encodes. nil `Inner` means JSON. |
| `Raw` | Stores `[]byte` and `string` as they are; other types go to `Inner`, nil means JSON. |

All three implement `Codec` with `Marshal` and `Unmarshal`.

### Backends

```go
type Backend interface {
	Get(ctx context.Context, key string) ([]byte, error)
	GetMany(ctx context.Context, keys ...string) ([][]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	SetMany(ctx context.Context, values map[string][]byte, ttl time.Duration) error
	SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	SetIfPresent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	Delete(ctx context.Context, key string) (bool, error)
	DeleteMany(ctx context.Context, keys ...string) error
	DeleteMatch(ctx context.Context, pattern string) error
	Scan(ctx context.Context, pattern string) iter.Seq2[string, error]
	Exists(ctx context.Context, key string) (bool, error)
	Incr(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Unlock(ctx context.Context, key string, owner []byte) (bool, error)
	ExtendLock(ctx context.Context, key string, owner []byte, ttl time.Duration) (bool, error)
	Ping(ctx context.Context) error
	Close() error
}

const NoExpiry time.Duration = -1

func WithPrefix(b Backend, prefix string) Backend
```

`Backend` stores bytes; implement it for your own storage and check it with `backendtest.Run`. The
methods match the basic API on bytes, plus:

| Method | Takes | Returns |
|---|---|---|
| `GetMany` | keys | values in the order of keys; a missing key gives a nil item. |
| `Scan` | a pattern | an iterator over matching keys; a failure is yielded once and the scan stops. |
| `Unlock` | the lock key, its owner | whether it removed the key: only if the key holds `owner`. |
| `ExtendLock` | the lock key, its owner, `ttl > 0` | whether it set the TTL: only if the key holds `owner`. |
| `Close` | - | releases what the backend owns. |

`NoExpiry` is what `TTL` returns for an entry that never expires. `WithPrefix` returns a backend that
keeps every key of `b` under `prefix`, for several services in one Redis; `Scan` yields keys without
it.

### Errors

Check them with `errors.Is`.

| Error | Returned when |
|---|---|
| `ErrNotFound` | a key has no live entry. |
| `ErrInvalidTTL` | a TTL is negative, or not positive where it must be. |
| `ErrWrongType` | `Incr` meets a value that is not an integer. |
| `ErrInvalidOptions` | a `Policy`, `BreakerOptions` or `Limit` is invalid. |
| `ErrLockLost` | `Lease.Extend` finds the lock expired. |
| `ErrRateLimited` | `RateLimit` refuses a call. |
| `ErrCircuitOpen` | `CircuitBreaker` refuses a call. |

## Package gashe/backend/memory

`import "github.com/eugeneliukindev/gashe/backend/memory"`

```go
func New(o Options) *Backend

type Options struct {
	MaxEntries    int
	MaxBytes      int64
	PurgeInterval time.Duration
}

var ErrTooLarge error

func (m *Backend) Len() int
func (m *Backend) Clear(context.Context) error
func (m *Backend) Close() error
```

`New` returns an LRU in process memory and starts its background purge; it panics on a negative
`MaxEntries` or `MaxBytes`. `Backend` implements `gashe.Backend`.

| Field | Zero value | Meaning |
|---|---|---|
| `MaxEntries` | 1000 | The most entries; the least recently used is evicted. |
| `MaxBytes` | no limit | The most bytes of keys and values; the least recently used are evicted. |
| `PurgeInterval` | 1s | How often expired entries are purged. Negative turns the purge off. |

`ErrTooLarge` is returned for a value that does not fit into `MaxBytes` alone. `Len` returns the
number of entries, expired ones not purged yet included. `Clear` removes everything, for tests.
`Close` stops the purge and always returns `nil`.

## Package gashe/backend/nearcache

`import "github.com/eugeneliukindev/gashe/backend/nearcache"`

```go
func New(local, shared gashe.Backend, o Options) *Backend

type Options struct {
	LocalTTL time.Duration
	Bus      Bus
}

type Bus interface {
	Publish(ctx context.Context, keys []string) error
	Subscribe(ctx context.Context, changed func(keys []string)) error
}
```

`New` returns a backend that reads through `local` (usually `memory`) in front of `shared` (usually
Redis). Writes and deletes go to `shared` and drop the local copy. `Backend` implements
`gashe.Backend`; its `Close` stops listening to the `Bus` and closes neither backend.

| Field | Zero value | Meaning |
|---|---|---|
| `LocalTTL` | 5 seconds | How long a local copy lives. |
| `Bus` | other processes see a change after `LocalTTL` | Tells the near caches of other processes which keys changed, so they drop their copies at once. |

`Bus` carries only key names. `Publish` sends them; nil means every key. `Subscribe` calls `changed`
with the keys others published, and `changed(nil)` when messages may have been lost; it returns when
`ctx` is done. `gasheredis.NewBus` is one over Redis pub/sub.

## Package gashe/backend/redis

`import gasheredis "github.com/eugeneliukindev/gashe/backend/redis"`, a separate module.

```go
func New(client goredis.UniversalClient) *Backend

func NewBus(client goredis.UniversalClient, channel string) *Bus
func (b *Bus) Publish(ctx context.Context, keys []string) error
func (b *Bus) Subscribe(ctx context.Context, changed func(keys []string)) error
```

`New` returns a backend over a go-redis v9 client: a single node, Sentinel or Cluster. You own the
client and close it; `Backend.Close` does nothing. `Backend` implements `gashe.Backend`. Errors are
wrapped as `gashe/redis: <op>: <error>`; a missing key becomes `gashe.ErrNotFound`, a wrong type
`gashe.ErrWrongType`.

`NewBus` returns a `nearcache.Bus` over Redis pub/sub on `channel`; use one channel per set of
processes that share the same keys. It panics on an empty channel. After every (re)subscription it
reports `changed(nil)`, and it retries every second while Redis is down.

## Package gashe/codec/msgpack

`import "github.com/eugeneliukindev/gashe/codec/msgpack"`, a separate module.

```go
type Codec struct{}
```

A `gashe.Codec` based on MessagePack. Use `gashe.Options{Codec: msgpack.Codec{}}`. Errors are wrapped
as `gashe/msgpack: marshal: <error>` and `gashe/msgpack: unmarshal: <error>`.

## Package gashe/backendtest

`import "github.com/eugeneliukindev/gashe/backendtest"`

```go
func Run(t *testing.T, newBackend func(t *testing.T) gashe.Backend)
```

Runs the contract suite against your backend. `newBackend` must return an empty backend for every
subtest.
