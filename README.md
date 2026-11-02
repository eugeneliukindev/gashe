<p align="center">
  <img src="docs/assets/logo.svg" width="200" alt="The gashe gopher, its cheeks full of cached data">
</p>

<h1 align="center">gashe</h1>

<p align="center">Caching for Go services that run as many processes.</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/eugeneliukindev/gashe"><img src="https://pkg.go.dev/badge/github.com/eugeneliukindev/gashe.svg" alt="Go Reference"></a>
  <a href="https://github.com/eugeneliukindev/gashe/tags"><img src="https://img.shields.io/github/v/tag/eugeneliukindev/gashe?filter=v*&label=release" alt="Release"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/eugeneliukindev/gashe" alt="Go version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/eugeneliukindev/gashe" alt="License"></a>
</p>

One `*gashe.Cache` stores results of expensive calls, keeps answering when a dependency is slow or
down, and guards dependencies with locks, rate limits and circuit breakers. Over Redis all of it is
shared by every pod.

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/backend/redis # Redis backend, on go-redis v9
go get github.com/eugeneliukindev/gashe/codec/msgpack # MessagePack codec, optional
```

Go 1.23 or newer. The core module has no third-party dependencies and no global state. What it
solves, problem by problem: [Strategies](#strategies).

## Usage example

```go
package main

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

type User struct {
	ID   int
	Name string
}

func main() {
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()

	backend := gasheredis.New(client)
	c := gashe.New(backend, gashe.Options{})
	defer c.Shutdown(ctx)

	u, err := gashe.GetOrLoad(ctx, c, "user:42", gashe.Policy[User]{
		TTL:          10 * time.Minute, // fresh for 10 minutes
		StaleIfError: time.Hour,        // the database is down: answer the old user for up to an hour
		LockOnMiss:   5 * time.Second,  // a cold key loads once for all pods
	}, func(ctx context.Context) (User, error) {
		return User{ID: 42, Name: "Ann"}, nil // your database query; runs on a miss only
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(u.Name)
}
```

Whole HTTP responses: [HTTP middleware](#http-middleware). One runnable program per feature:
[examples](examples/).

## Contents

[Configuration](#configuration) - [Backends](#backends) - [Keys and TTL](#keys-and-ttl) -
[API](#api) - [Strategies](#strategies) - [Cache invalidation](#cache-invalidation) -
[HTTP middleware](#http-middleware) - [Errors](#errors) - [Testing your code](#testing-your-code)

## Configuration

Every option struct works empty: a field you leave out gets its default. Create one `Cache` at
startup and share it; it is safe for concurrent use. It does not close the backend: close it where
you created it, after `c.Shutdown`.

```go
backend := memory.New(memory.Options{})
defer backend.Close() // stops the background purge
c := gashe.New(backend, gashe.Options{})
defer c.Shutdown(ctx) // runs before backend.Close: waits for background reloads
```

Every field of every option:

<details>
<summary><b>gashe.Options</b>: the cache</summary>

`c.CircuitBreaker(key, o)` takes the same `CircuitBreakerOptions`; there a nil `IsFailure` means any
error.

```go
c := gashe.New(backend, gashe.Options{
	Codec:            gashe.JSON{},          // values to bytes; nil means JSON
	OnError:          onError,               // errors worked around, not returned; nil drops them
	OnResult:         onResult,              // how each GetOrLoad was answered; nil drops them
	LockPollInterval: 20 * time.Millisecond, // how often Lock and LockOnMiss check a busy lock; 0 means 100ms
	Prefix:           "orders:",             // before every key, own keys included; "" means none
	CircuitBreaker: &gashe.CircuitBreakerOptions{ // stop calling a failing backend; nil means none
		FailurePercent: 50,               // opens once this share of calls failed, 1 to 99; required
		MinCalls:       20,               // calls the window needs before it may open; 0 means 1
		Window:         10 * time.Second, // the recent span calls are counted over; required
		OpenFor:        5 * time.Second,  // how long the open breaker refuses calls; required
		HalfOpenCalls:  1,                // test calls after OpenFor; 0 means it just closes
		IsFailure:      isFailure,        // which errors count; nil means any but a miss
	},
})
```

</details>

<details>
<summary><b>gashe.Policy</b>: one kind of data in GetOrLoad</summary>

```go
users := gashe.Policy[User]{
	TTL:                  time.Hour,              // how long a result is fresh; may be left out with a stale window
	Jitter:               0.1,                    // spreads TTLs by +-10%, from 0 to less than 1
	StaleWhileRevalidate: time.Minute,            // after TTL: answer the old result, reload in the background
	StaleIfError:         24 * time.Hour,         // after TTL: answer the old result instead of a loader error
	StaleIf:              isOutage,               // which loader errors StaleIfError covers; nil means any
	StaleIfSlow:          200 * time.Millisecond, // a slower loader gets the old result; needs StaleIfError
	LockOnMiss:           10 * time.Second,       // one process loads a miss, others wait; max hold
	Negative: gashe.Negative{ // cache "not found"; off while If is nil
		If:  isNotFound,      // which loader errors to cache
		Err: sql.ErrNoRows,   // what GetOrLoad returns while it is cached
		TTL: 5 * time.Minute, // how long it is cached
	},
	Tags:    []string{"users"},  // InvalidateTags of any of them makes the result a miss
	CacheIf: gashe.NotNil[User], // which results are stored; nil means every successful one
	Prefix:  "users:v2:",        // before the key, after Options.Prefix; "" means none
}
```

`TTL` takes a `time.Duration`, a string for `time.ParseDuration` or a function of the result:

```go
TTL: time.Hour
TTL: 90 * time.Second
TTL: "1h30m"
TTL: cfg.UserTTL // a string from the configuration, such as "10m"
TTL: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }
TTL: func(u User) time.Duration {
	if u.Admin {
		return time.Minute // admins change often
	}
	return time.Hour
}
TTL: func(p Page) time.Duration {
	if p.Draft {
		return -1 // negative: do not store
	}
	return 10 * time.Minute
}
TTL: 3600 // an int, not a time.Duration: GetOrLoad returns ErrInvalidOptions
```

</details>

<details>
<summary><b>gashe.SlidingWindow</b>, <b>gashe.FixedWindow</b>: Cache.RateLimit</summary>

```go
// each call counts one for the key; over the limit it returns gashe.ErrRateLimited
err := c.RateLimit(ctx, "api:"+userID, gashe.SlidingWindow{
	Calls: 100,         // about Calls in any span of Per; the same as gashe.PerMinute(100)
	Per:   time.Minute, // the span
})
err = c.RateLimit(ctx, "login:"+ip, gashe.FixedWindow{
	Calls:  5,                // calls per window, which starts with the first call
	Per:    time.Minute,      // the window
	BanFor: 15 * time.Minute, // how long the first call over the limit blocks the key; 0 means Per
})
```

</details>

<details>
<summary><b>memory.Options</b>: the memory backend</summary>

```go
backend := memory.New(memory.Options{
	MaxEntries:    10_000,      // entries, not bytes; the least recently used goes first; 0 means 1000
	MaxBytes:      256 << 20,   // bytes of keys and values, without the overhead of Go; 0 means no limit
	PurgeInterval: time.Minute, // how often expired entries are purged; 0 means every second, negative never
})
```

</details>

<details>
<summary><b>nearcache.Options</b>: memory in front of Redis</summary>

```go
backend := nearcache.New(local, shared, nearcache.Options{
	LocalTTL: 5 * time.Second,                        // how long a copy stays in memory; 0 means 5s
	Bus:      gasheredis.NewBus(client, "gashe:bus"), // drops copies elsewhere at once; nil: after LocalTTL
})
```

</details>

<details>
<summary><b>httpcache.Options</b>: the HTTP middleware</summary>

```go
pages := gashe.Policy[httpcache.Response]{TTL: time.Minute}
cached := httpcache.New(c, httpcache.Options{
	Policy:       pages,   // as in GetOrLoad; a nil CacheIf stores 200 only
	Key:          keyOf,   // nil: host, path and query, and no Authorization
	MaxBodyBytes: 4 << 20, // a larger body is sent but not stored; 0 means 1 MB
})
```

</details>

<details>
<summary><b>Codecs</b>: Options.Codec</summary>

```go
gashe.JSON{}                    // the default
gashe.Gzip{Inner: gashe.JSON{}} // compresses what Inner encodes; nil Inner means JSON
gashe.Raw{Inner: gashe.JSON{}}  // []byte and string as they are, the rest to Inner; basic API only
msgpack.Codec{}                 // codec/msgpack: smaller and faster than JSON
```

</details>

## Backends

<details>
<summary><b>Memory: in this process</b></summary>

An LRU with a TTL per entry, one per process. Values are copied in and out.

```go
import (
	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

backend := memory.New(memory.Options{MaxBytes: 256 << 20})
defer backend.Close() // stops the background purge
c := gashe.New(backend, gashe.Options{})
```

> **Warning:** `MaxEntries` counts entries, not bytes. Set `MaxBytes` with headroom: it does not
> count the overhead of Go. A value over `MaxBytes` gets `memory.ErrTooLarge`.

</details>

<details>
<summary><b>Redis: shared by every process</b></summary>

A separate module on [go-redis v9](https://github.com/redis/go-redis). You create and close the
client; timeouts, pool, TLS, Sentinel and Cluster are its options. Valkey, Dragonfly and KeyDB pass
the same tests.

```go
import (
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

client := goredis.NewUniversalClient(&goredis.UniversalOptions{
	Addrs:        []string{"localhost:6379"},
	ReadTimeout:  100 * time.Millisecond, // a slow cache must not slow the service down
	WriteTimeout: 100 * time.Millisecond,
})
defer client.Close()

backend := gasheredis.New(client)
c := gashe.New(backend, gashe.Options{})
```

- Entries are plain strings: read them with `redis-cli` and `DEL` them, never write them by hand.
- On a Cluster, multi-key calls go key by key; `Keys`, `All` and `DeleteMatch` visit every master.
- Errors read `gashe/redis: <op>: <error>`; a missing key is `gashe.ErrNotFound`.

> **Caution:** `Keys`, `All` and `DeleteMatch` scan the whole database. On a large Redis use
> `InvalidateTags`, and `redis-cli --scan`, not `KEYS *`.

</details>

<details>
<summary><b>Near cache: memory in front of Redis</b></summary>

See [Near cache](#near-cache).

</details>

<details>
<summary><b>Your own backend</b></summary>

Implement `gashe.Backend`: bytes only, the contract is the comments in `backend.go`. To change one
method, embed `gashe.Backend` and override it. Check it with the suite the bundled backends pass:

```go
import (
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backendtest"
)

func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // a fresh backend for every subtest
	})
}
```

</details>

## Keys and TTL

Parts are joined as they are, so put the separators into the prefixes yourself. Keys that start
with `_` belong to the cache; keys of yours must not:

```text
<Options.Prefix><Policy.Prefix><key>   your keys; the basic API and locks have no Policy.Prefix
<Options.Prefix>_<kind>:<key>          own keys; kind is load, refresh, tag, rate:fixed or rate:sliding
```

`_load:` and `_refresh:` are the `LockOnMiss` and reload locks of an entry, `_tag:` is the counter
of a tag, and `_rate:` are the rate limit counters.

In `GetOrLoad`, `TTL` is how long a result is fresh; the entry is kept `TTL` plus the longer of
`StaleWhileRevalidate` and `StaleIfError`. In the basic API a TTL of 0 means "never expires" and a
negative one returns `ErrInvalidTTL`.

> [!WARNING]
> Two call sites that read one key as different types make each other miss on every call. Give
> every type its own key or `Prefix`.

## API

Every call. [Strategies](#strategies) explain the options of `Policy`; the basic API is a map with
TTL over the same entries.

```go
// Create
c := gashe.New(backend, gashe.Options{}) // Options.Prefix, Options.CircuitBreaker and others
defer c.Shutdown(ctx) // waits for background reloads
c.Ping(ctx)
c.Backend() // the backend, with Options.Prefix and Options.CircuitBreaker applied

// Load through the cache
u, err := gashe.GetOrLoad(ctx, c, "42", policy, load)            // one key
users, err := gashe.GetOrLoadMany(ctx, c, ids, policy, loadMany) // loads only the missing keys
u, err = gashe.Reload(ctx, c, "42", policy, load)                // loads and stores, whatever is stored
c.InvalidateTags(ctx, "users")

// The basic API
c.Set(ctx, "key", v, time.Hour)          // ttl 0 = no expiry; replaces the old TTL
c.SetIfAbsent(ctx, "key", v, time.Hour)  // (bool, error)
c.SetIfPresent(ctx, "key", v, time.Hour) // (bool, error)
c.SetMany(ctx, map[string]any{"a": 1}, time.Hour)
c.Get(ctx, "key", &dst)                 // gashe.ErrNotFound on a miss
values, err := c.GetMany(ctx, "a", "b") // values[i].Found(), values[i].Decode(&dst)
for key, err := range c.Keys(ctx, "user:*") {}
for v, err := range c.All(ctx, "user:*") {} // v.Key(), v.Decode(&dst)
c.IncrBy(ctx, "counter", 1, time.Minute)    // ttl only when the counter is created
c.Exists(ctx, "key")
c.Delete(ctx, "key")
c.DeleteMany(ctx, "a", "b")
c.DeleteMatch(ctx, "user:*")
c.Expire(ctx, "key", time.Minute)
c.TTL(ctx, "key") // gashe.NoExpiry for no expiry

// Protect dependencies
lease, err := c.Lock(ctx, "job", time.Minute)        // waits; lease.Unlock(ctx), lease.Extend(ctx, ttl)
lease, ok, err := c.TryLock(ctx, "job", time.Minute) // does not wait
c.IsLocked(ctx, "job")
c.RateLimit(ctx, "login:"+ip, gashe.PerMinute(5))   // gashe.ErrRateLimited over the limit
done, err := c.CircuitBreaker("db", breakerOptions) // gashe.ErrCircuitOpen while open; then done(err)
```

<details>
<summary>The same calls in Redis</summary>

What the Redis backend sends for each call. Keys of several calls go one command each in a
pipeline, so they may live in different Cluster slots.

| Call | Redis |
|---|---|
| `Get` | `GET` |
| `GetMany` | `GET` per key in a pipeline, like `MGET` |
| `Set` | `SET`, with `EX` or `PX` for a TTL |
| `SetMany` | `SET` per key in a pipeline, like `MSET` |
| `SetIfAbsent` | `SET NX` |
| `SetIfPresent` | `SET XX` |
| `IncrBy` | `INCRBY`; with a TTL, `SET NX` with the TTL and `INCRBY` in `MULTI` |
| `Exists` | `EXISTS` |
| `Delete` | `UNLINK` |
| `DeleteMany` | `UNLINK` per key in a pipeline |
| `DeleteMatch` | `SCAN` and `UNLINK` |
| `Keys`, `All` | `SCAN`; `All` also reads the keys in batches, like `GetMany` |
| `Expire` | `PEXPIRE` |
| `TTL` | `PTTL` |
| `Ping` | `PING` |

</details>

Patterns are Redis globs: `*`, `?`, `[abc]`, `[^a]`, `[a-z]`, `\` to escape. Values are anything
the codec can encode; with JSON, exported fields only. `GetOrLoad` returns the value as it reads
back from the cache, so a field that is lost shows up on the first call, not only on a hit.

Example: [examples/basic](examples/basic/main.go).

## Strategies

Each section names a problem caches run into and the call or option that solves it, with a
runnable program in [examples](examples/). Most are fields of one `gashe.Policy[V]` and can be
combined. Over `memory` everything stays inside one process; over Redis it is shared by every
process that uses that Redis, unless a section says otherwise.

| Problem | Known as | Solution |
|---|---|---|
| The same expensive call again and again | [Memoization](https://en.wikipedia.org/wiki/Memoization) | [`GetOrLoad`](#cache-aside) |
| Every request waits when a popular key expires | [Cache stampede](https://en.wikipedia.org/wiki/Cache_stampede) on a hot key | [`StaleWhileRevalidate`](#stale-while-revalidate) |
| The database is down and the cache is empty | [Graceful degradation](https://en.wikipedia.org/wiki/Graceful_degradation) | [`StaleIfError`](#stale-if-error) |
| The database is slow, and every request waits for it | [Graceful degradation](https://en.wikipedia.org/wiki/Graceful_degradation) | [`StaleIfSlow`](#stale-if-error) |
| Lookups of missing ids always reach the database | [Negative cache](https://en.wikipedia.org/wiki/Negative_cache), cache penetration | [`Negative`](#negative-caching) |
| A heavy page is rendered on every request | [Web cache](https://en.wikipedia.org/wiki/Web_cache) | [`httpcache`](#http-middleware) |
| One change makes many entries stale | [Cache invalidation](https://en.wikipedia.org/wiki/Cache_invalidation) | [`Tags`, `InvalidateTags`](#invalidation-by-tag) |
| A cold key: a thousand identical queries | [Cache stampede](https://en.wikipedia.org/wiki/Cache_stampede), dog-piling | built in for one process, [`LockOnMiss`](#cache-stampede-across-processes) for all pods |
| N+1 queries on a list page | N+1 query problem | [`GetOrLoadMany`](#batch-loading) |
| Keys stored together expire together | [Thundering herd](https://en.wikipedia.org/wiki/Thundering_herd_problem), cache avalanche | [`Jitter`](#ttl-jitter) |
| A hot key costs a round trip on every read | Hot key | [`nearcache`](#near-cache) |
| Work must run once at a time across pods | [Mutual exclusion](https://en.wikipedia.org/wiki/Mutual_exclusion), [distributed lock](https://en.wikipedia.org/wiki/Distributed_lock_manager) | [`Lock`, `TryLock`](#distributed-lock) |
| A client calls far too often | [Rate limiting](https://en.wikipedia.org/wiki/Rate_limiting) | [`RateLimit`](#rate-limiting) |
| A failing dependency drags the service down | [Cascading failure](https://en.wikipedia.org/wiki/Cascading_failure) | [`CircuitBreaker`](#circuit-breaker) |
| Redis is down, and every request waits for its timeout | [Cascading failure](https://en.wikipedia.org/wiki/Cascading_failure) | [`Options.CircuitBreaker`](#cache-failure) |
| A deploy changed a cached type | [Schema evolution](https://en.wikipedia.org/wiki/Schema_evolution) | built in: [shapes](#type-changes) |

How `GetOrLoad` answers, with the options that change each step:

```text
GetOrLoad
 |- fresh .................................. answer: hit
 |- stale, within StaleWhileRevalidate ..... answer, reload in the background
 |- missing, with LockOnMiss ............... one process loads, the others wait
 '- otherwise .............................. run the loader
      |- ok ................................ store and answer
      '- error, or slower than StaleIfSlow
           |- stale, within StaleIfError ... answer the stale entry
           '- otherwise .................... return the error
```

### Caching

#### Cache-aside

The same expensive call is made again and again. `GetOrLoad` returns the stored result while it is
fresh; otherwise it calls your loader, stores the result for `TTL` and returns it. A loader error
is returned as is and never stored.

```go
u, err := gashe.GetOrLoad(ctx, c, "user:42", gashe.Policy[User]{TTL: time.Hour}, func(ctx context.Context) (User, error) {
	return loadUser(ctx, 42) // runs only when the cache cannot answer
})
```

Use the `ctx` the loader receives, not the one around it: a background reload calls the loader
after the request has ended. Three more fields fit most cases:

```go
gashe.Policy[[]Item]{TTL: time.Minute, CacheIf: func(v []Item) bool { return len(v) > 0 }} // skip empty results
gashe.Policy[Token]{TTL: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }}
gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"} // stores "users:v2:42"
```

Example: [examples/cacheaside](examples/cacheaside/main.go).

#### Stale-while-revalidate

A popular key expires, and every request waits for the slow loader. `StaleWhileRevalidate` works as
the directive of [RFC 5861](https://www.rfc-editor.org/rfc/rfc5861#section-3): for this long after
`TTL` the stale result is answered at once while one caller reloads it in the background; with
Redis, one caller across all processes.

```go
feed, err := gashe.GetOrLoad(ctx, c, "feed:"+userID, gashe.Policy[[]Item]{
	TTL:                  5 * time.Minute,
	StaleWhileRevalidate: 30 * time.Minute,
}, loadFeed)
```

Stored like a cache-aside entry, but for `TTL + StaleWhileRevalidate` (35 minutes). While one
process reloads it, `_refresh:feed:u1` holds the random id of that process, such as
`f4fefbc6731d4abccff956cae8ca012f`, for at most a minute.

> [!CAUTION]
> The background reload runs after the request has ended: its `ctx` keeps the values of the
> request but not its cancellation, and gets a one-minute deadline. Do not use request-scoped
> resources, such as an open transaction, inside the loader.

Example: [examples/stale](examples/stale/main.go).

#### Stale-if-error

The database or an API is down, and expired entries leave nothing to answer with. `StaleIfError`
works as the directive of [RFC 5861](https://www.rfc-editor.org/rfc/rfc5861#section-4): for this
long after `TTL` a stale result answers a loader error; the error goes to `OnError`. `StaleIf`
picks which errors count.

```go
rates, err := gashe.GetOrLoad(ctx, c, "rates:usd", gashe.Policy[Rates]{
	TTL:          time.Minute,
	StaleIfError: 6 * time.Hour,
	StaleIf:      func(err error) bool { return errors.Is(err, ErrAPIDown) }, // nil = any error
}, loadRates)
```

Stored for `TTL + StaleIfError` (6 hours and a minute), though fresh only for the minute in `at`.

A source that is slow rather than down is covered by `StaleIfSlow`: when the loader takes longer,
the call answers with the stale result at once (`ResultStaleIfSlow`), and the load goes on in the
background and stores what it gets. While it runs, other calls also get the stale result instead of
starting more loads, with Redis across all pods.

```go
gashe.Policy[Rates]{TTL: time.Minute, StaleIfError: 6 * time.Hour, StaleIfSlow: 200 * time.Millisecond}
```

> [!WARNING]
> An answer can be up to `TTL + StaleIfError` old. Do not use it where a stale answer is wrong
> rather than late: permissions, balances, prices at checkout.

Example: [examples/stale](examples/stale/main.go).

#### Negative caching

Requests for data that does not exist (a scraper walking ids) are never cached, so all of them
reach the database. `Negative` caches "not found" for a short while and returns the error you chose
while it is cached.

```go
gashe.Negative{If: func(err error) bool { return errors.Is(err, sql.ErrNoRows) }, Err: sql.ErrNoRows, TTL: 30 * time.Second}
```

> [!WARNING]
> Data created after a negative answer stays "not found" until `Negative.TTL` ends. Keep it short,
> or `Delete` the key when you create the data.

Example: [examples/negative](examples/negative/main.go).

### Load on the source

#### Cache stampede inside a process

A key is missing and a thousand goroutines ask for it at once. Built in: concurrent `GetOrLoad`
calls with the same key and type in one process share one lookup and one load. This sharing lives
in process memory and writes nothing to Redis; with 50 pods the key still loads 50 times.

#### Cache stampede across processes

The same with 50 pods: 50 identical queries. With `LockOnMiss`, on a miss one process takes a lock
in Redis and loads; the others wait and read its result. The key loads once for all pods. The value
is the longest the lock is held, in case its holder dies.

```go
stats, err := gashe.GetOrLoad(ctx, c, "stats:"+day, gashe.Policy[Stats]{
	TTL:        10 * time.Minute,
	LockOnMiss: time.Minute,
}, loadStats)
```

While one pod loads, `_load:stats:2026-11-02` holds the random id of that pod for at most
`LockOnMiss`; it is deleted once the result is stored.

> [!WARNING]
> Make `LockOnMiss` longer than the slowest load, or a second pod starts loading when the lock
> expires. Waiting calls wait as long as their `ctx` allows.

Example: [examples/stampede](examples/stampede/main.go).

#### Batch loading

A page shows 50 products: 50 cache round trips and, on a cold cache, 50 queries. `GetOrLoadMany`
reads all keys in one call and calls the loader once with the missing ones; keys the loader leaves
out are not stored. It shares entries with `GetOrLoad` and supports `TTL`, `Jitter`,
`Tags`, `CacheIf` and `Prefix`; the other options return `ErrInvalidOptions`.

```go
products, err := gashe.GetOrLoadMany(ctx, c, ids, gashe.Policy[Product]{TTL: time.Hour, Prefix: "product:"},
	func(ctx context.Context, missing []string) (map[string]Product, error) {
		return s.repo.ProductsByID(ctx, missing) // SELECT ... WHERE id = ANY($1)
	})
```

Concurrent calls are not shared: two requests for the same cold list both call the loader.

Example: [examples/batch](examples/batch/main.go).

#### TTL jitter

Keys stored together (after a deploy or a warm-up) expire together, and the database gets all of
their loads at once (cache avalanche). `Jitter` spreads every TTL randomly by up to this share.

```go
gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.2} // 48 to 72 minutes
```

Products stored at the same moment get TTLs such as 3009, 4183 and 3263 seconds instead of 3600
each.

#### Near cache

A hot key is read thousands of times a second, each read a network round trip to one Redis node.
`nearcache` keeps a copy in process memory for `LocalTTL`; writes and deletes go to Redis and drop
the local copy. Locks, counters and TTLs always use Redis.

With a `Bus`, a pod that changes a key tells the others, and they drop their copies at once instead
of after `LocalTTL`. Only the key name is sent; the next read takes the new value from Redis.
`InvalidateTags` becomes instant on every pod too.

```go
import (
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

local := memory.New(memory.Options{MaxBytes: 64 << 20})
defer local.Close()
shared := gasheredis.New(client)

backend := nearcache.New(local, shared, nearcache.Options{
	LocalTTL: time.Minute,
	Bus:      gasheredis.NewBus(client, "orders:invalidate"), // one channel per service
})
defer backend.Close() // stops listening to the bus
c := gashe.New(backend, gashe.Options{})
```

> [!WARNING]
> Every pod keeps its own copy, so with 50 pods the local cache takes RAM 50 times: bound it with
> `MaxBytes`. The bus sees only changes made through gashe; a key changed by `redis-cli` or another
> service stays stale up to `LocalTTL`. While a pod is disconnected from Redis it may miss
> messages; it drops all its copies when it reconnects.

Example: [examples/nearcache](examples/nearcache/main.go).

### Storage

#### Key prefixes

`Options.Prefix` puts every key of a service under one prefix, so services share one Redis:
`orders:` stores `orders:user:42`, see [keys](#keys-and-ttl). Keep environments in separate Redis
servers, not in the prefix.

#### Type changes

A deploy renames a field of a cached type, and `encoding/json` reads old entries without an error,
leaving the new field empty until they expire. Built in: every entry keeps `t`, a hash of the shape
of its type (exported field names, json tags and types), and an entry of another shape is a miss
that loads again. Only types that changed reload; changing values does not change the shape.

```go
type User struct{ Name string }     // stored with "t":1273...
type User struct{ FullName string } // after the deploy: another shape, so old entries are misses
```

#### Large values

Big values take memory in Redis and network on every read. `Gzip` compresses what another codec
encodes; `codec/msgpack` is a smaller binary format; `Raw` stores `[]byte` and `string` as they are
in the basic API.

```go
c := gashe.New(backend, gashe.Options{Codec: gashe.Gzip{}}) // a 320 KB page takes 2 KB in Redis
```

> [!CAUTION]
> Changing the codec, like changing `Prefix`, makes every stored entry a miss at once. `IncrBy` does
> not work on compressed or MessagePack values.

### Protecting dependencies

#### Distributed lock

Some work must never run twice at once across your pods: a daily report, a payment. `Lock` and
`TryLock` take a lock in Redis; the TTL frees it if the holder dies, `Extend` keeps it longer.

```go
lease, ok, err := c.TryLock(ctx, "job:reindex", time.Minute) // Lock waits instead
if err != nil || !ok {
	return err // Redis is down, or another pod holds it
}
defer lease.Unlock(ctx)              // releases it only if it is still yours
err = lease.Extend(ctx, time.Minute) // for long work; gashe.ErrLockLost if it expired
```

Stored under `job:reindex`: the random id of the holder, such as
`621708739092f7d5ef73f2358d22a804`, for the lock TTL.

> [!CAUTION]
> Locks are advisory: a holder paused longer than the TTL (GC, a frozen VM) can overlap with the
> next one. Make writes that guard money idempotent too.

Example: [examples/lock](examples/lock/main.go).

#### Rate limiting

A client or a bug calls an expensive endpoint far too often. `RateLimit` counts a call and returns
`ErrRateLimited` once the key is over its limit; with Redis the limit is shared by all pods, with
`memory` each pod counts only its own calls. If the backend fails, the call is let through.

```go
err := c.RateLimit(ctx, "api-key:"+key, gashe.PerMinute(600)) // sliding window
err = c.RateLimit(ctx, "login:"+ip, gashe.FixedWindow{Calls: 5, Per: time.Minute, BanFor: 15 * time.Minute})
if errors.Is(err, gashe.ErrRateLimited) {
	http.Error(w, "too many requests", http.StatusTooManyRequests)
}
```

A sliding window stores a plain counter per span of `Per`, such as `3` under
`_rate:sliding:api-key:k1:29893701`, each for twice `Per`. A fixed window is one counter under
`_rate:fixed:login:1.2.3.4`; the first call over the limit sets its TTL to `BanFor`.

`SlidingWindow` is an estimate and can let slightly more through under bursts; `FixedWindow` can
let up to twice `Calls` through around the border of two windows.

Example: [examples/ratelimit](examples/ratelimit/main.go).

#### Circuit breaker

A dependency starts failing, every request still waits for it, and the failure spreads. The breaker
counts calls of the last `Window`; once `FailurePercent` of them fail, it refuses calls with
`ErrCircuitOpen` for `OpenFor`, then lets `HalfOpenCalls` test calls through. It lives in process
memory: each pod judges the dependency by its own calls, and Redis holds nothing.

```go
done, err := c.CircuitBreaker("payments-api", gashe.CircuitBreakerOptions{
	FailurePercent: 50, MinCalls: 10, Window: 30 * time.Second, OpenFor: 15 * time.Second,
})
if err != nil {
	return err // gashe.ErrCircuitOpen: answer at once instead of waiting for a timeout
}
done(provider.Charge(ctx, p)) // report once: nil or the error
```

> [!WARNING]
> The state of every key is kept for the life of the `Cache`: use a few fixed keys, not user ids
> or URLs.

Example: [examples/breaker](examples/breaker/main.go).

### Operations

#### Cache failure

Built in: if the backend fails, `GetOrLoad` returns the loader result without storing it,
`RateLimit` lets the call through, and the error goes to `OnError`. Bound how long a slow Redis can
hold a request with go-redis `ReadTimeout` and `WriteTimeout`.

When Redis is down, every call still waits for that timeout before it goes to the loader.
`Options.CircuitBreaker` stops calling a failing backend for a while: calls then fail at once with
`ErrCircuitOpen`, and `GetOrLoad` goes straight to the loader. Misses do not count as failures,
and `Ping` always reaches Redis.

```go
c := gashe.New(backend, gashe.Options{
	CircuitBreaker: &gashe.CircuitBreakerOptions{
		FailurePercent: 50, MinCalls: 20, Window: 10 * time.Second, OpenFor: 5 * time.Second, HalfOpenCalls: 1,
	},
})
```

> [!WARNING]
> With Redis down, `LockOnMiss` no longer holds other pods back, and every pod sends its misses to
> the database. Put a `CircuitBreaker` in front of the database for that case.

#### Monitoring

`OnResult` receives an `Outcome` for every `GetOrLoad` and every key of `GetOrLoadMany`: the
key, how it was answered (hit, miss, stale-while-revalidate, stale-if-error, stale-if-slow or
negative-hit) and how long the loader ran, 0 on a hit. It sees the calls of this process; sum the
metrics of all pods.

```go
OnResult: func(ctx context.Context, o gashe.Outcome) {
	results.WithLabelValues(o.Result.String()).Inc() // hit ratio
	loads.Observe(o.Load.Seconds())                  // what a miss costs: pick TTLs by it
},
```

Both run synchronously on every call: keep them fast.

Example: [examples/monitoring](examples/monitoring/main.go).

#### Graceful shutdown

A background reload may still run when the service stops and write to a closed backend.
`Shutdown` stops starting reloads and waits for the running ones; call it before closing Redis.

```go
_ = srv.Shutdown(ctx) // the HTTP server first
_ = c.Shutdown(ctx)
_ = client.Close()
```

#### Known limits

- **Cache-aside race with `Delete`.** A read can load an old value, a write can update the
  database and delete the key, and then the read stores its old value until its TTL ends. Tags do
  not have this race, see [invalidation by tag](#invalidation-by-tag).
- **Locks are advisory**, see [distributed lock](#distributed-lock).
- **Clocks.** Freshness compares the clock of the reading process with the time the writer stored.
  Keep clocks in sync (NTP).

## Cache invalidation

### Invalidation by tag

One change makes many entries stale, and deleting them by pattern scans the whole of Redis. Give
entries `Tags`; `InvalidateTags` makes every entry with the tag a miss with one write per tag.

```go
profile, err := gashe.GetOrLoad(ctx, c, "profile:42", gashe.Policy[Profile]{TTL: time.Hour, Tags: []string{"user:42"}}, loadProfile)
orders, err := gashe.GetOrLoad(ctx, c, "orders:42", gashe.Policy[[]Order]{TTL: time.Hour, Tags: []string{"user:42", "orders"}}, loadOrders)

_, err = db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = 42", name)
err = c.InvalidateTags(ctx, "user:42") // after the write, not before
```

A tag is a counter: `InvalidateTags` raises it, and entries stored with an older value miss.
Nothing is deleted, old entries expire by their TTL. It also closes the race of `Delete`, since a
read takes the counters before it loads. For data that must not be stale after a write, give each
item its own tag, such as `"user:" + id`: one more key read per lookup.

> [!NOTE]
> Tag counters never expire: every tag name is one small key forever. Tags per user are fine;
> tags per request are not.

Example: [examples/tags](examples/tags/main.go).

### Other ways to invalidate

```go
gashe.Reload(ctx, c, "user:42", gashe.Policy[User]{TTL: time.Hour}, loadUser) // load and store a new result now
c.Delete(ctx, "user:42")
c.DeleteMatch(ctx, "user:*") // scans the whole backend: avoid on a large Redis
```

`Delete` leaves the race above open: a read that loaded before the write can store its old value
after the `Delete`. Use a tag where that matters.

To drop everything of one kind, change its `Prefix`. Every key then misses at once: expect the load
of a cold cache after such a deploy. A changed type does not need it, see
[type changes](#type-changes).

## HTTP middleware

`httpcache` caches whole responses of a handler: a hit sends the stored status, headers and body
without calling the handler. It runs on `GetOrLoad`, so every `Policy` field works for pages: a 5xx
with `StaleIfError` gets the last good page, `LockOnMiss` renders a cold page once for all pods.

```go
import (
	"net/http"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/httpcache"
)

cached := httpcache.New(c, httpcache.Options{Policy: gashe.Policy[httpcache.Response]{
	Prefix:       "http:",
	TTL:          time.Minute,
	StaleIfError: time.Hour,
	Tags:         []string{"catalog"}, // c.InvalidateTags(ctx, "catalog") after an edit
}})
mux.Handle("GET /products/", cached(http.HandlerFunc(listProducts)))
```

It follows the HTTP standards a shared cache must:

- **What is stored**, as [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111#section-3) asks of a
  shared cache: only `GET` and `HEAD` without `Range`, never responses with `Set-Cookie`, `Vary` or
  `Cache-Control` `no-store`, `private` or `no-cache`, and never requests with `Authorization`
  unless your `Key` includes the user. Hop-by-hop headers are not stored
  ([RFC 9110](https://www.rfc-editor.org/rfc/rfc9110#section-7.6.1)).
- **Conditional requests**: every response gets an `ETag`, and a matching `If-None-Match` gets
  `304 Not Modified` without a body, with the weak comparison of
  [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110#section-13.1.2).
- **`Cache-Status`** ([RFC 9211](https://www.rfc-editor.org/rfc/rfc9211)) says how the request was
  handled: `gashe; hit`, `gashe; fwd=uri-miss; stored`, `gashe; fwd=uri-miss`, `gashe; fwd=method`
  or `gashe; fwd=request`. A hit also gets `Age`.

Two rules are stricter on purpose: request `Cache-Control` and `Pragma` are ignored, so clients
cannot make the handler run on every request, and how long a page is fresh comes from `Policy`, not
from `max-age`.

> [!CAUTION]
> A response must be the same for everyone who gets its key. If a page depends on a cookie, set
> `Key` and put the user in it, or do not wrap that route.

Example: [examples/httpserver](examples/httpserver/main.go).

## Errors

Check them with `errors.Is`.

<details>
<summary>Every error</summary>

| Error | When |
|---|---|
| `ErrNotFound` | `Get`, `TTL`, `Value.Decode` on a miss |
| `ErrInvalidTTL` | a negative TTL, or a non-positive one where a lock or `Expire` needs it |
| `ErrInvalidOptions` | a `Policy`, `Limit` or `CircuitBreakerOptions` that cannot work |
| `ErrWrongType` | `IncrBy` on a value that is not an integer |
| `ErrLockLost` | `Lease.Extend` of a lock that expired |
| `ErrRateLimited` | `RateLimit` refuses a call |
| `ErrCircuitOpen` | `CircuitBreaker` or `Options.CircuitBreaker` refuses a call |

</details>

Loader errors are returned unchanged. `GetOrLoad` and `RateLimit` never return backend errors:
they work around them and send them to `OnError`. `Lock`, `TryLock`, `Extend` and
`InvalidateTags` do return them.

## Testing your code

Use the memory backend with the purge off, so no goroutine outlives the test:

```go
func newCache(t *testing.T) *gashe.Cache {
	backend := memory.New(memory.Options{PurgeInterval: -1}) // no purge goroutine: nothing to close
	return gashe.New(backend, gashe.Options{})
}
```

TTLs follow `time.Now`, so on Go 1.25+ `testing/synctest` runs them in fake time; `synctest.Wait()`
lets background reloads finish. Leave `Jitter` out where a test checks exact TTLs. To simulate a
failing backend, embed a real one and override a method:

```go
type downBackend struct{ gashe.Backend }

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
```

## More

- [API reference](https://pkg.go.dev/github.com/eugeneliukindev/gashe) on pkg.go.dev.
- [Changelog](CHANGELOG.md) of every release.
- [Glossary](docs/GLOSSARY.md) of the terms used here, and [Contributing](CONTRIBUTING.md).

Under the [MIT license](LICENSE). The gopher in the logo is based on the Go gopher by
[Renee French](https://reneefrench.blogspot.com/), licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/), and was redrawn for gashe.
