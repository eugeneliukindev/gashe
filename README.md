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
down, and guards dependencies with locks, rate limits and circuit breakers. Over a shared backend, such as
Redis, all of it is shared by every pod.

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/backend/redis # Redis backend, on go-redis v9
go get github.com/eugeneliukindev/gashe/codec/msgpack # MessagePack codec, optional
go get github.com/eugeneliukindev/gashe/compress/zstd # Zstandard compressor, optional
```

Go 1.23 or newer. The core module has no third-party dependencies and no global state.

# Usage example

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // the Postgres driver for database/sql
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
	"github.com/eugeneliukindev/gashe/codec/msgpack"
	"github.com/eugeneliukindev/gashe/compress/zstd"
)

type User struct {
	ID   int
	Name string
}

func main() {
	ctx := context.Background()
	db, err := sql.Open("pgx", "postgres://localhost:5432/app")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()

	backend := gasheredis.New(client)
	c := gashe.New(backend, gashe.Options{
		Codec:      msgpack.Codec{}, // the format of stored values; JSON when left out
		Compressor: zstd.New(),      // shrinks them before they reach Redis; none when left out
	})
	defer c.Shutdown(ctx)

	userCache := gashe.For(c, gashe.Policy[User]{
		TTL:          10 * time.Minute, // fresh for 10 minutes
		StaleIfError: time.Hour,        // the database is down: answer the old user for up to an hour
		LockOnMiss:   5 * time.Second,  // a cold key loads once for all pods
		Prefix:       "user:",          // stores user:42
	})

	u, err := userCache.GetOrLoad(ctx, "42", func(ctx context.Context) (User, error) { // runs on a miss only
		var u User
		err := db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE id = $1", 42).Scan(&u.ID, &u.Name)
		return u, err
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(u.Name)
}
```

One runnable program per feature: [examples](examples/).

# What it solves

| Problem | Known as | Solution |
|---|---|---|
| The same expensive call again and again | [Memoization](https://en.wikipedia.org/wiki/Memoization) | [`GetOrLoad`](#getorload) |
| Every request waits when a popular key expires | [Cache stampede](https://en.wikipedia.org/wiki/Cache_stampede) on a hot key | [`StaleWhileRevalidate`](#stalewhilerevalidate-answer-old-data-while-reloading) |
| The database is down and the cache is empty | [Graceful degradation](https://en.wikipedia.org/wiki/Graceful_degradation) | [`StaleIfError`](#staleiferror-staleifslow-answer-old-data-while-the-source-fails) |
| The database is slow, and every request waits for it | [Graceful degradation](https://en.wikipedia.org/wiki/Graceful_degradation) | [`StaleIfSlow`](#staleiferror-staleifslow-answer-old-data-while-the-source-fails) |
| Lookups of missing ids always reach the database | [Negative cache](https://en.wikipedia.org/wiki/Negative_cache), cache penetration | [`Negative`](#negative-cache-not-found) |
| A heavy page is rendered on every request | [Web cache](https://en.wikipedia.org/wiki/Web_cache) | [`httpcache`](#httpcache-whole-http-responses) |
| One change makes many entries stale | [Cache invalidation](https://en.wikipedia.org/wiki/Cache_invalidation) | [`Tags`, `InvalidateTags`](#tags-invalidate-many-entries-with-one-write) |
| A cold key: a thousand identical queries | [Cache stampede](https://en.wikipedia.org/wiki/Cache_stampede), dog-piling | built in for one process, [`LockOnMiss`](#lockonmiss-one-load-for-all-pods) for all pods |
| N+1 queries on a list page | N+1 query problem | [`GetOrLoadMany`](#getorloadmany) |
| Keys stored together expire together | [Thundering herd](https://en.wikipedia.org/wiki/Thundering_herd_problem), cache avalanche | [`Jitter`](#jitter-spread-expiry) |
| A hot key costs a round trip on every read | Hot key | [`nearcache`](#backendnearcache) |
| Work must run once at a time across pods | [Mutual exclusion](https://en.wikipedia.org/wiki/Mutual_exclusion), [distributed lock](https://en.wikipedia.org/wiki/Distributed_lock_manager) | [`WithLock`, `TryLock`](#locks) |
| A client calls far too often | [Rate limiting](https://en.wikipedia.org/wiki/Rate_limiting) | [`RateLimit`](#cratelimit) |
| A failing dependency drags the service down | [Cascading failure](https://en.wikipedia.org/wiki/Cascading_failure) | [`Guard`, `CircuitBreaker`](#gasheguard) |
| The cache backend is down, and every request waits for its timeout | [Cascading failure](https://en.wikipedia.org/wiki/Cascading_failure) | [`Options.CircuitBreaker`](#circuitbreaker-keep-working-when-the-backend-is-down) |
| A deploy changed a cached type | [Schema evolution](https://en.wikipedia.org/wiki/Schema_evolution) | built in: [shapes](#type-shapes-type-changes-after-a-deploy) |

# Contents

- [Setup](#setup): `gashe.New` and every option, `c.Shutdown`, `c.Ping`, `c.Backend`
- [Backends](#backends): memory, Redis, near cache, your own, testing your code
- [Load through the cache](#load-through-the-cache): [`gashe.For`](#gashefor),
  [`GetOrLoad`](#getorload) and every option of its policy, [`GetOrLoadMany`](#getorloadmany),
  [`Reload`](#reload), [`Cached.Set`](#cachedset), [`Cached.Delete`](#cacheddelete),
  [`httpcache`](#httpcache-whole-http-responses)
- [Locks](#locks): `c.WithLock`, `c.TryLock`, `c.Lock`, `lease.Extend`, `lease.Unlock`
- [Limits and breakers](#limits-and-breakers): `c.RateLimit`, `gashe.Guard`, `c.CircuitBreaker`
- [Values by key](#values-by-key): `c.Set`, `c.Get`, `c.GetMany` and the rest of the basic API
- [Keys and counters](#keys-and-counters): `c.Keys`, `c.All`, `c.DeleteMatch`, `c.IncrBy`,
  `c.Expire`, `c.TTL`
- [Errors](#errors), [how gashe stores data](#how-gashe-stores-data), [benchmarks](#benchmarks)

**Redis:** in a card lists the commands the Redis backend sends; Valkey and the other compatible
servers get the same.

# Setup

## `gashe.New`

```go
c := gashe.New(backend, gashe.Options{ // every field may be left out
	Prefix:           "orders:",             // before every key, own keys included; "" means none
	Codec:            gashe.JSON{},          // the format of values; nil means JSON
	Compressor:       zstd.New(),            // shrinks what Codec produced; nil means none
	CircuitBreaker:   &cacheBreaker,         // stop calling a failing backend; nil means none
	OnError:          onError,               // errors worked around, not returned; nil drops them
	OnResult:         onResult,              // how each GetOrLoad was answered; nil drops them
	LockPollInterval: 20 * time.Millisecond, // how often Lock and LockOnMiss check; 0 means 100ms
})
```

- **Does:** creates the cache over a backend.
- **Errors:** panics on a nil backend, a negative `LockPollInterval` or invalid
  `Options.CircuitBreaker`.
- **Note:** create one at startup and share it; it does not close the backend.

### `Prefix`: several services in one backend

```go
c := gashe.New(backend, gashe.Options{Prefix: "orders:"}) // stores orders:user:42
```

- **Does:** puts every key of the cache under the prefix, own keys included.
- **Note:** `Keys`, `All` and `DeleteMatch` never reach past it; keep environments in separate servers,
  not in the prefix. Key templates: [keys](#keys).

---

### `Codec`: the format of values

```go
gashe.JSON{}                      // the default, in the core: readable with redis-cli
msgpack.Codec{}                   // codec/msgpack, a separate module: binary, smaller, faster to decode
raw.Codec{Inner: msgpack.Codec{}} // codec/raw: []byte and string as they are, the rest to Inner
```

- **Does:** turns a value into bytes and back; which to choose: [benchmarks](#benchmarks).
- **Note:** changing the codec makes every entry a miss at once; `raw.Codec` helps the basic API only;
  `IncrBy` does not work on MessagePack values.

---

### `Compressor`: large values

```go
zstd.New() // compress/zstd: a separate module
gzip.New() // compress/gzip: in the core module, the standard library only
```

- **Problem:** big values take memory in the backend and network on every read.
- **Fix:** shrinks the bytes of the codec, with any codec; both store about the same, zstd is faster.
  Which to choose: [benchmarks](#benchmarks).
- **Note:** create one at startup and share it; changing it makes every entry a miss at once;
  `IncrBy` does not work on compressed values.

---

### `CircuitBreaker`: keep working when the backend is down

```go
cacheBreaker := gashe.CircuitBreakerOptions{FailurePercent: 50, MinCalls: 20, Window: 10 * time.Second, OpenFor: 5 * time.Second}
```

- **Problem:** with the backend down, every call waits for its timeout before it goes to the loader.
- **Fix:** stops calling a failing backend for a while: calls fail at once with `ErrCircuitOpen`,
  and `GetOrLoad` goes straight to the loader.
- **Note:** built in without it: a failing backend never fails `GetOrLoad` or `RateLimit`; bound how
  long a slow Redis holds a request with go-redis `ReadTimeout`; a nil `IsFailure` here does not
  count a miss; `Ping` is not guarded.

> [!WARNING]
> With the backend down, `LockOnMiss` no longer holds other pods back, and every pod sends its misses
> to the database. Put a breaker in front of the database for that case.

---

### `OnResult`, `OnError`: monitoring

```go
OnResult: func(ctx context.Context, o gashe.Outcome) {
	results.WithLabelValues(o.Result.String()).Inc() // hit ratio
	loads.Observe(o.Load.Seconds())                  // what a miss costs: pick TTLs by it
},
```

- **Does:** `OnResult` gets an `Outcome` for every `GetOrLoad` and every key of `GetOrLoadMany`: the
  key, how it was answered, and how long the loader ran; `OnError` gets the errors the cache works
  around.
- **Note:** both run synchronously on every call, so keep them fast; each sees its own process.
- **Example:** [examples/monitoring](examples/monitoring/main.go)

## `c.Shutdown`

```go
_ = srv.Shutdown(ctx) // the HTTP server first
_ = c.Shutdown(ctx)
_ = client.Close() // the backend last
```

- **Does:** stops starting background reloads and waits for the running ones.
- **Errors:** `ctx.Err()` - `ctx` ended first.
- **Note:** call it before closing the backend; the cache keeps working afterwards, without
  background reloads.

## `c.Ping`

```go
err := c.Ping(ctx)
```

- **Does:** checks that the backend is reachable, past `Options.CircuitBreaker`.
- **Redis:** `PING`.

## `c.Backend`

```go
b := c.Backend()
```

- **Returns:** the backend, with `Options.Prefix` and `Options.CircuitBreaker` applied.

# Backends

## `backend/memory`

```go
backend := memory.New(memory.Options{
	MaxEntries:    10_000,      // entries, not bytes; the least recently used goes first; 0 means 1000
	MaxBytes:      256 << 20,   // bytes of keys and values, without the overhead of Go; 0 means no limit
	PurgeInterval: time.Minute, // how often expired entries are purged; 0 means every second, negative never
})
defer backend.Close() // stops the background purge
```

- **Does:** an LRU in process memory with a TTL per entry, one per process.
- **Errors:** `memory.ErrTooLarge` - a value over `MaxBytes`.
- **Note:** values are copied in and out; set `MaxBytes` with headroom.

## `backend/redis`

```go
client := goredis.NewUniversalClient(&goredis.UniversalOptions{
	Addrs:        []string{"localhost:6379"},
	ReadTimeout:  100 * time.Millisecond, // a slow cache must not slow the service down
	WriteTimeout: 100 * time.Millisecond,
})
defer client.Close()
backend := gasheredis.New(client)
```

- **Does:** a backend on [go-redis v9](https://github.com/redis/go-redis), shared by every process;
  a separate module.
- **Errors:** `gashe/redis: <op>: <error>`; a missing key is `gashe.ErrNotFound`.
- **Note:** timeouts, pool and TLS are options of the client; read entries with `redis-cli --scan`,
  never write them by hand. It works with these servers:
  - Redis 7.0 and newer, Valkey 8: tested on every release;
  - Dragonfly, KeyDB: pass the same tests; Dragonfly lacks `CLIENT KILL`, so the reconnect test of
    `Bus` is skipped there;
  - Redis Cluster and Sentinel: through `UniversalClient`; on a Cluster, calls on several keys go
    key by key, and `SCAN` visits every master;
  - managed Redis and Valkey (ElastiCache, MemoryDB, Azure Cache for Redis, Memorystore, Upstash):
    not tested; they need `SET` with `NX`, `XX` and `PX`, `UNLINK`, `SCAN`, `MULTI`, Lua scripts for
    locks and pub/sub for `Bus`.

## `backend/nearcache`

```go
local := memory.New(memory.Options{MaxBytes: 64 << 20})
defer local.Close()

backend := nearcache.New(local, gasheredis.New(client), nearcache.Options{
	LocalTTL: time.Minute,                                    // how long a copy stays in memory; 0 means 5s
	Bus:      gasheredis.NewBus(client, "orders:invalidate"), // one channel per service; nil: after LocalTTL
})
defer backend.Close() // stops listening to the bus
```

- **Problem:** a hot key is read thousands of times a second, each read a round trip to the shared
  backend.
- **Does:** keeps a copy in process memory for `LocalTTL`; with a `Bus`, a change on one pod drops
  the copies on the others at once.
- **Note:** writes, locks, counters and TTLs go to the shared backend; every pod keeps its own copy, so bound it
  with `MaxBytes`; a key changed outside gashe stays old up to `LocalTTL`.
- **Example:** [examples/nearcache](examples/nearcache/main.go)

## Your own backend

```go
func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // a fresh backend for every subtest
	})
}
```

- **Does:** implement `gashe.Backend`; the contract is the comments in `backend.go`.
- **Note:** to change one method, embed `gashe.Backend` and override it; check it with the suite the
  bundled backends pass.

## Testing your code

```go
func newCache(t *testing.T) *gashe.Cache {
	backend := memory.New(memory.Options{PurgeInterval: -1}) // no purge goroutine: nothing to close
	return gashe.New(backend, gashe.Options{})
}

type downBackend struct{ gashe.Backend } // a failing backend: embed a real one, override a method

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
```

- **Does:** a cache for tests, and a backend that fails.
- **Note:** on Go 1.25+ `testing/synctest` runs TTLs in fake time, and `synctest.Wait()` lets
  background reloads finish; leave `Jitter` out where a test checks exact TTLs.

# Load through the cache

## `gashe.For`

```go
userCache := gashe.For(c, gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"})
```

- **Does:** binds a `Policy` to the cache and checks it once.
- **Returns:** a `gashe.Cached[User]`, whose methods are the functions of the same name with the
  policy.
- **Errors:** panics on an invalid policy, such as `TTL: "1hr"`.
- **Note:** its `Set` and `Delete` apply `Policy.Prefix`; keep one per kind of data.

## `GetOrLoad`

```go
u, err := userCache.GetOrLoad(ctx, "42", loadUser)           // userCache := gashe.For(c, userPolicy)
u, err = gashe.GetOrLoad(ctx, c, "42", userPolicy, loadUser) // the same without For
```

- **Does:** answers the stored result while the policy allows; otherwise calls the loader, stores
  the result and returns it.
- **Returns:** the result as it reads back from the cache.
- **Errors:** the loader error, unless `StaleIfError` answers it. `Negative.Err` - a "not found" is
  cached. `ctx.Err()` - `ctx` ended while `LockOnMiss` waited. `ErrInvalidOptions` - an invalid
  policy, in the function form.
- **Redis:** `GET` of the key and its `_tag:` counters in one pipeline; `SET ... PX` to store.
- **Note:** concurrent calls for one key and type in a process share one load; backend errors go to
  `OnError`, and the loader answers; the loader gets a `ctx` that may outlive the request.
- **Example:** [examples/cacheaside](examples/cacheaside/main.go)

### `Policy`: every field

```go
userPolicy := gashe.Policy[User]{
	TTL:                  time.Hour,              // how long a result is fresh
	Jitter:               0.1,                    // spreads TTLs by +-10%, from 0 to less than 1
	StaleWhileRevalidate: time.Minute,            // after TTL: answer the old result, reload in the background
	StaleIfError:         24 * time.Hour,         // after TTL: answer the old result instead of a loader error
	StaleIf:              isOutage,               // which loader errors StaleIfError covers; nil means any
	StaleIfSlow:          200 * time.Millisecond, // a slower loader gets the old result; needs StaleIfError
	Negative: gashe.Negative{ // cache "not found"; off while If is nil
		If:  isNotFound,      // which loader errors to cache
		Err: sql.ErrNoRows,   // what GetOrLoad returns while it is cached
		TTL: 5 * time.Minute, // how long it is cached
	},
	LockOnMiss: 10 * time.Second,   // one process loads a miss, the others wait; the longest hold
	Tags:       []string{"users"},  // InvalidateTags of any of them makes the result a miss
	CacheIf:    gashe.NotNil[User], // which results are stored; nil means every successful one
	Prefix:     "users:v2:",        // before the key, after Options.Prefix; "" means none
}
```

- **Does:** says how `GetOrLoad` caches the results of one type; a zero field is off.
- **Note:** define it once, bind it with `gashe.For`, and share it.

---

### `TTL`: how long a result is fresh

```go
TTL: time.Hour
TTL: "1h30m"
TTL: cfg.UserTTL // a string from the configuration, such as "10m"
TTL: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }
TTL: func(p Page) time.Duration {
	if p.Draft {
		return -1 // negative: do not store
	}
	return 10 * time.Minute
}
TTL: 3600 // an int, not a time.Duration: ErrInvalidOptions
```

- **Does:** takes a `time.Duration`, a string for `time.ParseDuration`, or a function of the result
  that gashe calls after a load.
- **Note:** the entry lives for `TTL` plus the longer of `StaleWhileRevalidate` and `StaleIfError`;
  a TTL that depends on the arguments needs no function: `TTL: ttlFor(itemID)`.

---

### `Jitter`: spread expiry

```go
gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.2} // 48 to 72 minutes each
```

- **Problem:** keys stored together expire together, and the database gets all their loads at once.
- **Fix:** spreads every TTL randomly by up to this share.

---

### `StaleWhileRevalidate`: answer old data while reloading

```go
feedCache := gashe.For(c, gashe.Policy[[]Item]{TTL: 5 * time.Minute, StaleWhileRevalidate: 30 * time.Minute})
```

- **Problem:** a popular key expires, and every request waits for the loader.
- **Fix:** for this long after `TTL`, answers the old result and reloads it in the background, one
  caller across all pods ([RFC 5861](https://www.rfc-editor.org/rfc/rfc5861#section-3)).
- **Redis:** `SET _refresh:<key> NX PX` for the reload.
- **Note:** the reload runs after the request has ended, with a one-minute deadline.
- **Example:** [examples/stale](examples/stale/main.go)

> [!CAUTION]
> Do not use an open transaction or other request-scoped resources in the loader: the reload
> outlives the request.

---

### `StaleIfError`, `StaleIfSlow`: answer old data while the source fails

```go
rateCache := gashe.For(c, gashe.Policy[Rates]{
	TTL:          time.Minute,
	StaleIfError: 6 * time.Hour,
	StaleIf:      func(err error) bool { return errors.Is(err, ErrAPIDown) }, // nil = any error
	StaleIfSlow:  200 * time.Millisecond,                                     // needs StaleIfError
})
```

- **Problem:** the database or an API is down or slow, and expired entries leave nothing to answer.
- **Fix:** for `StaleIfError` after `TTL`, the old result answers a loader error
  ([RFC 5861](https://www.rfc-editor.org/rfc/rfc5861#section-4)); a loader slower than
  `StaleIfSlow` gets the old result at once, and the load goes on in the background.
- **Note:** the error goes to `OnError`; `StaleIf` picks which errors count; while a slow load runs,
  other calls get the old result too.
- **Example:** [examples/stale](examples/stale/main.go)

> [!WARNING]
> A result can be up to `TTL + StaleIfError` old. Do not use it where old is wrong rather than late:
> permissions, balances, prices at checkout.

---

### `Negative`: cache "not found"

```go
gashe.Policy[User]{TTL: time.Hour, Negative: gashe.Negative{
	If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
	Err: sql.ErrNoRows,
	TTL: 30 * time.Second,
}}
```

- **Problem:** lookups of ids that do not exist are never cached, and every one reaches the database.
- **Fix:** caches the loader error for `Negative.TTL` and returns `Negative.Err` meanwhile.
- **Note:** data created meanwhile stays "not found" until `Negative.TTL` ends: keep it short, or
  delete the key when you create the data.
- **Example:** [examples/negative](examples/negative/main.go)

---

### `LockOnMiss`: one load for all pods

```go
statsCache := gashe.For(c, gashe.Policy[Stats]{TTL: 10 * time.Minute, LockOnMiss: time.Minute})
```

- **Problem:** a shared load is built in for one process only: 50 pods send 50 identical queries.
- **Fix:** on a miss, one process takes a lock and loads; the others wait and read its result.
- **Redis:** `SET _load:<key> NX PX`, and a script that releases it.
- **Note:** make it longer than the slowest load, or a second pod loads when the lock expires;
  waiting calls wait as long as their `ctx` allows.
- **Example:** [examples/stampede](examples/stampede/main.go)

---

### `Tags`: invalidate many entries with one write

```go
profileCache := gashe.For(c, gashe.Policy[Profile]{TTL: time.Hour, Tags: []string{"user:42"}})
orderCache := gashe.For(c, gashe.Policy[[]Order]{TTL: time.Hour, Tags: []string{"user:42", "orders"}})

_, err = db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = 42", name)
err = c.InvalidateTags(ctx, "user:42") // after the write, not before
```

- **Problem:** one change makes many entries stale, and deleting them by pattern scans the whole backend.
- **Fix:** `c.InvalidateTags` makes every entry with the tag a miss; nothing is deleted.
- **Errors:** `InvalidateTags` returns backend errors.
- **Redis:** `INCRBY _tag:<tag> 1` per tag.
- **Note:** a tag per item costs one more key read per lookup; tag counters never expire, so tag per
  user, not per request.
- **Example:** [examples/tags](examples/tags/main.go)

---

### `CacheIf`, `Prefix`: what to store and under which key

```go
gashe.Policy[[]Item]{TTL: time.Minute, CacheIf: func(v []Item) bool { return len(v) > 0 }} // skip empty results
gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"}                                    // stores users:v2:42
```

- **Does:** `CacheIf` decides which results are stored; `Prefix` goes before every key of the policy.
- **Note:** changing `Prefix` makes every entry of that kind a miss at once.

---

### Type shapes: type changes after a deploy

```go
type User struct{ Name string }     // before the deploy
type User struct{ FullName string } // after it: old entries are misses
```

- **Problem:** a deploy renames a field, and `encoding/json` reads old entries without an error,
  leaving the new field empty.
- **Fix:** built in: an entry of another type shape is a miss that loads again.
- **Note:** only changed types reload; values do not change the shape.

## `GetOrLoadMany`

```go
list, err := productCache.GetOrLoadMany(ctx, ids, func(ctx context.Context, missing []string) (map[string]Product, error) {
	return repo.ProductsByID(ctx, missing) // SELECT ... WHERE id = ANY($1)
})
list, err = gashe.GetOrLoadMany(ctx, c, ids, productPolicy, loadProducts) // the same without For
```

- **Problem:** a page shows 50 products: 50 cache round trips and, on a cold cache, 50 queries.
- **Does:** reads all keys at once and calls the loader once with the missing ones.
- **Returns:** a map of every key that was found or loaded.
- **Errors:** the loader error. `ErrInvalidOptions` - `StaleWhileRevalidate`, `StaleIfError`,
  `LockOnMiss` or `Negative` is set.
- **Redis:** `GET` per key and per tag in one pipeline; `SET ... PX` per loaded key in one pipeline.
- **Note:** keys the loader leaves out are not stored; concurrent calls do not share a load.
- **Example:** [examples/batch](examples/batch/main.go)

## `Reload`

```go
u, err := userCache.Reload(ctx, "42", loadUser)
u, err = gashe.Reload(ctx, c, "42", userPolicy, loadUser)
```

- **Does:** calls the loader and stores the result, whatever is stored.
- **Returns:** the loaded result.
- **Errors:** the loader error; the stored entry stays.
- **Redis:** `GET` of the tag counters, then `SET ... PX`.
- **Note:** use it after the data changed when you do not have the new result.

## `Cached.Set`

```go
_, err = db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = 42", u.Name)
err = userCache.Set(ctx, "42", u) // the next GetOrLoad is a hit with the new result
```

- **Does:** stores a result you already have, as a load would: with the TTL, `Jitter`, tags and
  shape of the policy.
- **Errors:** backend and codec errors.
- **Redis:** `GET` of the tag counters, then `SET users:v2:42 ... PX`.
- **Note:** a result the policy does not store, by `CacheIf` or a negative TTL, deletes the entry
  (`UNLINK`); unlike `Delete`, it has no race with a parallel load.

## `Cached.Delete`

```go
existed, err := userCache.Delete(ctx, "42") // deletes users:v2:42
```

- **Does:** deletes the entry, with `Policy.Prefix`.
- **Returns:** whether the entry existed.
- **Redis:** `UNLINK`.
- **Note:** a read that loaded before the write can store its old result after the delete; use `Set`
  or a tag where that matters.

## `httpcache`: whole HTTP responses

```go
cached := httpcache.New(c, httpcache.Options{
	Policy: gashe.Policy[httpcache.Response]{ // as in GetOrLoad; a nil CacheIf stores 200 only
		TTL:          time.Minute,
		StaleIfError: time.Hour,
		Tags:         []string{"catalog"}, // c.InvalidateTags(ctx, "catalog") after an edit
		Prefix:       "http:",
	},
	Key:          keyOf,   // the key of a request; nil: host, path and query, no Authorization
	MaxBodyBytes: 4 << 20, // a larger body is sent but not stored; 0 means 1 MB
})
mux.Handle("GET /products/", cached(http.HandlerFunc(listProducts)))
```

- **Problem:** a heavy page is rendered on every request.
- **Does:** answers a hit with the stored status, headers and body, without calling the handler.
- **Note:** it runs on `GetOrLoad`, so every `Policy` field works for pages, and it follows the
  standards of a shared cache:
  - **Stored** ([RFC 9111](https://www.rfc-editor.org/rfc/rfc9111#section-3)): `GET` and `HEAD`
    without `Range` only; never `Set-Cookie`, `Vary` or `Cache-Control` `no-store`, `private`,
    `no-cache`; never `Authorization` unless `Key` includes the user; hop-by-hop headers dropped
    ([RFC 9110](https://www.rfc-editor.org/rfc/rfc9110#section-7.6.1)).
  - **Conditional** ([RFC 9110](https://www.rfc-editor.org/rfc/rfc9110#section-13.1.2)): every
    response gets an `ETag`; a matching `If-None-Match` gets `304 Not Modified`.
  - **`Cache-Status`** ([RFC 9211](https://www.rfc-editor.org/rfc/rfc9211)): `gashe; hit`,
    `gashe; fwd=uri-miss; stored`, `gashe; fwd=uri-miss`, `gashe; fwd=method` or
    `gashe; fwd=request`; a hit also gets `Age`.
  - **Stricter on purpose:** request `Cache-Control` and `Pragma` are ignored; freshness comes from
    `Policy`, not `max-age`.
- **Example:** [examples/httpserver](examples/httpserver/main.go)

> [!CAUTION]
> A response must be the same for everyone who gets its key. If a page depends on a cookie, put the
> user into `Key`, or do not wrap that route.

# Locks

Some work must never run twice at once across pods: a daily report, a payment. A lock lives in the
backend and is shared by every process; its TTL frees it if the holder dies.

> [!CAUTION]
> Locks are advisory: a holder paused longer than the TTL (GC, a frozen VM) can overlap with the next
> one. Make writes that guard money idempotent too.

## `c.WithLock`

```go
err := c.WithLock(ctx, "job:reindex", time.Minute, func(ctx context.Context) error {
	return reindex(ctx)
})
```

- **Does:** waits for the lock, runs the function, extends the lock every third of the TTL while it
  runs, and releases it.
- **Returns:** the error of the function.
- **Errors:** `ErrLockLost` - an extension failed, and the `ctx` of the function is cancelled.
  `ctx.Err()` - the wait ended first. `ErrInvalidTTL` - the TTL is not positive.
- **Redis:** as `c.Lock`, `lease.Extend` and `lease.Unlock`.
- **Example:** [examples/lock](examples/lock/main.go)

## `c.TryLock`

```go
lease, ok, err := c.TryLock(ctx, "job:reindex", time.Minute)
if err != nil || !ok {
	return err // the backend is down, or another pod runs the job: skip it
}
defer lease.Unlock(ctx)
```

- **Does:** takes the lock if it is free, without waiting.
- **Returns:** `ok` is false and `lease` is nil while someone else holds it.
- **Errors:** as `c.Lock`.
- **Redis:** `SET key <random id> NX PX`.

## `c.Lock`

```go
lease, err := c.Lock(ctx, "job:reindex", time.Minute)
defer lease.Unlock(ctx)
```

- **Does:** takes the lock, waiting while someone else holds it.
- **Returns:** a `*gashe.Lease`.
- **Errors:** `ctx.Err()` - `ctx` ended first. `ErrInvalidTTL` - the TTL is not positive. Backend
  errors.
- **Redis:** `SET key <random id> NX PX`, every `LockPollInterval` while the lock is busy.
- **Note:** use it for a lock that outlives one function; otherwise use `WithLock`.

## `lease.Extend`

```go
err := lease.Extend(ctx, time.Minute)
```

- **Does:** gives the lock a new TTL from now, if it is still yours.
- **Errors:** `ErrLockLost` - it expired, and someone else may hold it. `ErrInvalidTTL` - the TTL is
  not positive.
- **Redis:** a script that checks the holder id and runs `PEXPIRE`.

## `lease.Unlock`

```go
err := lease.Unlock(ctx)
```

- **Does:** releases the lock if it is still yours.
- **Redis:** a script that checks the holder id and runs `DEL`.
- **Note:** it ignores the cancellation of `ctx`, so a finished request does not leave the lock behind.

## `c.IsLocked`

```go
locked, err := c.IsLocked(ctx, "job:reindex")
```

- **Returns:** whether someone holds the lock.
- **Redis:** `EXISTS`.

# Limits and breakers

## `c.RateLimit`

```go
err := c.RateLimit(ctx, "login:"+ip, gashe.FixedWindow{
	Calls:  5,                // calls per window, which starts with the first call
	Per:    time.Minute,      // the window
	BanFor: 15 * time.Minute, // how long the first call over the limit blocks the key; 0 means Per
})
err = c.RateLimit(ctx, "api-key:"+key, gashe.PerMinute(600)) // = SlidingWindow{Calls: 600, Per: time.Minute}

var limited *gashe.RateLimitError // errors.Is(err, gashe.ErrRateLimited) holds too
if errors.As(err, &limited) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
	http.Error(w, "too many requests", http.StatusTooManyRequests)
}
```

- **Problem:** a client or a bug calls an expensive endpoint far too often.
- **Does:** counts one call for the key across all pods, and refuses calls over the limit.
- **Errors:** `*RateLimitError` - over the limit; it is `ErrRateLimited` and has `RetryAfter`.
  `ErrInvalidOptions` - the limit cannot work.
- **Redis:** `FixedWindow`: `INCRBY _rate:fixed:<key>` with the TTL `Per`, then `PEXPIRE` to `BanFor`
  or `PTTL`. `SlidingWindow`: `INCRBY` of the current window, `GET` of the last one, `INCRBY -1` when
  refused.
- **Note:** call it before the work it guards; a failing backend lets the call through; `RetryAfter`
  assumes no other calls come; `SlidingWindow` is an estimate, and `FixedWindow` can let twice `Calls`
  through at the border of two windows.
- **Example:** [examples/ratelimit](examples/ratelimit/main.go)

## `gashe.Guard`

```go
paymentsBreaker := gashe.CircuitBreakerOptions{
	FailurePercent: 50,               // opens once this share of calls failed, 1 to 99; required
	MinCalls:       10,               // calls the window needs before it may open; 0 means 1
	Window:         30 * time.Second, // the recent span calls are counted over; required
	OpenFor:        15 * time.Second, // how long the open breaker refuses calls; required
	HalfOpenCalls:  1,                // test calls after OpenFor; 0 means it just closes
	IsFailure:      isFailure,        // which errors count; nil means any
}
receipt, err := gashe.Guard(c, "payments-api", paymentsBreaker, func() (Receipt, error) {
	return provider.Charge(ctx, p)
})
```

- **Problem:** a dependency starts failing, every request still waits for it, and the failure spreads.
- **Does:** runs one call through the breaker of the key and reports its result.
- **Returns:** what the call returned.
- **Errors:** `ErrCircuitOpen` - the breaker is open, and the call does not run. `ErrInvalidOptions` -
  the options cannot work.
- **Redis:** none: the breaker lives in process memory.
- **Note:** each pod judges by its own calls; the state of every key lives as long as the `Cache`, so
  use a few fixed keys, not user ids.
- **Example:** [examples/breaker](examples/breaker/main.go)

## `c.CircuitBreaker`

```go
done, err := c.CircuitBreaker("payments-api", paymentsBreaker)
if err != nil {
	return err
}
done(provider.Refund(ctx, p)) // once: nil or the error of the call
```

- **Does:** checks the breaker before a call you report yourself.
- **Returns:** a `DoneFunc` to call once with the result.
- **Errors:** as `gashe.Guard`.
- **Redis:** none.
- **Note:** use it when the call does not fit one function; otherwise use `Guard`.

# Values by key

The basic API: plain values, a TTL in every call, no `Policy`. It does not read entries of
`GetOrLoad`: keep one key on one side.

## `c.Set`

```go
err := c.Set(ctx, "key", v, time.Hour) // ttl 0 = no expiry
```

- **Does:** stores the value, replacing the entry and its TTL.
- **Errors:** `ErrInvalidTTL` - the TTL is negative; codec and backend errors.
- **Redis:** `SET`, with `EX` or `PX` for a TTL.
- **Example:** [examples/basic](examples/basic/main.go)

## `c.SetIfAbsent`

```go
stored, err := c.SetIfAbsent(ctx, "key", v, time.Hour)
```

- **Does:** stores the value only if the key has no entry.
- **Returns:** whether it stored.
- **Errors:** as `c.Set`.
- **Redis:** `SET ... NX PX`; `SETNX` without a TTL.

## `c.SetIfPresent`

```go
stored, err := c.SetIfPresent(ctx, "key", v, time.Hour)
```

- **Does:** stores the value only if the key has an entry.
- **Returns:** whether it stored.
- **Errors:** as `c.Set`.
- **Redis:** `SET ... XX`.

## `c.SetMany`

```go
err := c.SetMany(ctx, map[string]any{"a": 1, "b": 2}, time.Hour)
```

- **Does:** stores all values with one TTL.
- **Errors:** as `c.Set`.
- **Redis:** `SET` per key in one pipeline.

## `c.Get`

```go
err := c.Get(ctx, "key", &dst)
```

- **Does:** decodes the stored value into `dst`.
- **Errors:** `ErrNotFound` - a miss; a decode error; backend errors.
- **Redis:** `GET`.

## `c.GetMany`

```go
values, err := c.GetMany(ctx, "a", "b")
if values[0].Found() {
	err = values[0].Decode(&dst) // values[0].Key() is "a"
}
```

- **Does:** reads the keys in one call.
- **Returns:** one `Value` per key, in order; a missing key gives a value that is not found.
- **Errors:** backend errors; `Decode` of a value not found returns `ErrNotFound`.
- **Redis:** `GET` per key in one pipeline.

## `c.Delete`

```go
existed, err := c.Delete(ctx, "key")
```

- **Does:** deletes the key as it is, without `Policy.Prefix`.
- **Returns:** whether the key existed.
- **Redis:** `UNLINK`.

## `c.DeleteMany`

```go
err := c.DeleteMany(ctx, "a", "b")
```

- **Does:** deletes the keys; missing keys are fine.
- **Redis:** `UNLINK` per key in one pipeline.

# Keys and counters

Patterns are Redis globs: `*`, `?`, `[abc]`, `[^a]`, `[a-z]`, `\` to escape.

## `c.Keys`

```go
for key, err := range c.Keys(ctx, "user:*") {
	if err != nil {
		return err
	}
	fmt.Println(key) // without Options.Prefix
}
```

- **Does:** yields every key that matches the pattern.
- **Errors:** a failure is yielded once, and the scan stops.
- **Redis:** `SCAN MATCH` on every Cluster master.
- **Note:** it walks the whole database and may yield a key twice; on a large backend use tags.

## `c.All`

```go
for v, err := range c.All(ctx, "user:*") {
	if err != nil {
		return err
	}
	_ = v.Decode(&dst) // v.Key() is the key
}
```

- **Does:** yields every live value whose key matches the pattern.
- **Errors:** as `c.Keys`.
- **Redis:** `SCAN`, then `GET` of the found keys in pipelines of 100.

## `c.DeleteMatch`

```go
err := c.DeleteMatch(ctx, "user:*")
```

- **Does:** deletes every key that matches the pattern, never past `Options.Prefix`.
- **Redis:** `SCAN`, then `UNLINK` in pipelines of 100.
- **Note:** it walks the whole database; on a large backend use tags.

## `c.IncrBy`

```go
n, err := c.IncrBy(ctx, "visits", 1, time.Hour) // by may be negative
```

- **Does:** adds `by` to the integer under the key; creates it from 0.
- **Returns:** the new value.
- **Errors:** `ErrWrongType` - the value is not an integer. `ErrInvalidTTL` - the TTL is negative. An
  error on int64 overflow.
- **Redis:** `INCRBY`; with a TTL, `SET key 0 NX PX` and `INCRBY` in one `MULTI`.
- **Note:** the TTL applies only when the counter is created; later calls neither extend nor reset it.

## `c.Exists`

```go
ok, err := c.Exists(ctx, "key")
```

- **Returns:** whether the key has a live entry.
- **Redis:** `EXISTS`.

## `c.Expire`

```go
existed, err := c.Expire(ctx, "key", time.Minute)
```

- **Does:** sets a new TTL from now.
- **Returns:** whether the key existed.
- **Errors:** `ErrInvalidTTL` - the TTL is not positive.
- **Redis:** `PEXPIRE`.

## `c.TTL`

```go
left, err := c.TTL(ctx, "key")
```

- **Returns:** the time left; `gashe.NoExpiry` for a key without a TTL.
- **Errors:** `ErrNotFound` - the key is missing.
- **Redis:** `PTTL`.

# Errors

Check them with `errors.Is`.

| Error | When |
|---|---|
| `ErrNotFound` | `Get`, `TTL`, `Value.Decode` on a miss |
| `ErrInvalidTTL` | a negative TTL, or a non-positive one where a lock or `Expire` needs it |
| `ErrInvalidOptions` | a `Policy`, `Limit` or `CircuitBreakerOptions` that cannot work |
| `ErrWrongType` | `IncrBy` on a value that is not an integer |
| `ErrLockLost` | `Lease.Extend` of a lock that expired |
| `ErrRateLimited` | `RateLimit` refuses a call; the error is a `*RateLimitError` with `RetryAfter` |
| `ErrCircuitOpen` | `CircuitBreaker` or `Options.CircuitBreaker` refuses a call |


Loader errors are returned unchanged. `GetOrLoad` and `RateLimit` never return backend errors:
they work around them and send them to `OnError`. `Lock`, `TryLock`, `Extend` and
`InvalidateTags` do return them.

# How gashe stores data

What lands in the backend, for reading Redis with `redis-cli` and for reasoning about edge cases.
The format of entries is internal: read it, `DEL` it, never write it by hand.

- [Keys](#keys), [TTL of an entry](#ttl-of-an-entry), [the GetOrLoad entry](#the-getorload-entry)
- [What each strategy stores](#what-each-strategy-stores), [tags](#tags), [type shapes](#type-shapes)
- [Known limits](#known-limits)

## Keys

Parts are joined as they are, so put the separators into the prefixes yourself. Keys that start
with `_` belong to the cache; keys of yours must not:

```text
<Options.Prefix><Policy.Prefix><key>   your keys; the basic API and locks have no Policy.Prefix
<Options.Prefix>_<kind>:<key>          own keys; kind is load, refresh, tag, rate:fixed or rate:sliding
```

- `_load:` and `_refresh:` are the `LockOnMiss` and reload locks of an entry.
- `_tag:` is the counter of a tag.
- `_rate:` are the rate limit counters.

## TTL of an entry

- In `GetOrLoad`, `TTL` is how long a result is fresh. The entry is kept for `TTL` plus the longer
  of `StaleWhileRevalidate` and `StaleIfError`, then the backend drops it.
- In the basic API a TTL of 0 means "never expires", and a negative one returns `ErrInvalidTTL`.

## The GetOrLoad entry

One string per key: an entry that the codec encodes, JSON by default.

```text
{"at":"2026-11-03T13:02:54.221386728+03:00","v":{"ID":42,"Name":"Ann"},"t":15130343655375510895}
```

- `at`: when the result stops being fresh.
- `v`: the result.
- `t`: the shape of the type of `v`, see [type shapes](#type-shapes).
- `g`: the counters of its tags when it was stored, with `Tags` only.
- `neg`: `true` for a cached "not found", with `Negative` only.

The basic API stores the value itself, without this wrapper, so `c.Get` cannot read a `GetOrLoad`
entry.

## What each strategy stores

| Strategy | Key | Value | TTL |
|---|---|---|---|
| `GetOrLoad` | `user:42` | the entry above | `TTL` + the longer window |
| `StaleWhileRevalidate` reload | `_refresh:feed:u1` | the random id of the reloading process | at most a minute |
| `LockOnMiss` | `_load:stats:2026-11-02` | the random id of the loading process | `LockOnMiss`, deleted once stored |
| `Lock`, `TryLock`, `WithLock` | `job:reindex` | the random id of the holder | the lock TTL |
| `Tags` | `_tag:users` | a number | none |
| `SlidingWindow` | `_rate:sliding:api-key:k1:29893701` | the calls of one span of `Per` | twice `Per` |
| `FixedWindow` | `_rate:fixed:login:1.2.3.4` | the calls of the window | `Per`; `BanFor` after the first call over |
| `CircuitBreaker` | nothing: it lives in process memory | | |

## Tags

- A tag is a counter. An entry keeps the counters of its tags from when it was stored; a lookup reads
  them again, and an entry with an older counter is a miss.
- `InvalidateTags` raises the counters: one write per tag, no scan, nothing deleted.
- A missing counter, never created or evicted, gets a random value. Reading it as 0 would make
  entries invalidated before the eviction valid again.
- A read takes the counters before it loads, so a slow read cannot store an old value after a write
  and `InvalidateTags`.

## Type shapes

`t` is a hash of the shape of the type: exported field names, json tags and field types. A lookup
whose type has another shape is a miss that loads again, so a renamed field does not read as empty
after a deploy. Values do not change the shape. Entries without `t`, stored by gashe v0.1.0, are read
as they are.

## Known limits

- **The race of `Delete`.** A read loads an old value, a write updates the database and deletes the
  key, then the read stores its old value until its TTL ends. `Cached.Set` and tags do not have this
  race.
- **Locks are advisory.** A holder paused longer than the TTL (GC, a frozen VM) can overlap with the
  next one.
- **Clocks.** Freshness compares the clock of the reading process with the time the writer stored.
  Keep clocks in sync (NTP).
- **One key, one type.** Two call sites that read one key as different types make each other miss on
  every call. Give every type its own key or `Prefix`.

# Benchmarks

Encoding and decoding with every codec and compressor, the median of 5 runs of `just bench -count=5`
on an Intel Core i9-9900KF, Go 1.26, linux/amd64. The code is in [benchmarks](benchmarks/). The data
is generated from 12 words, so it compresses better than real text does.

**A user**, 4 fields:

| Codec + compressor | Size | Encode | Decode |
|---|---|---|---|
| json | 68 B | 238 ns | 1.1 us |
| json + gzip | 92 B | 16.4 us | 2.4 us |
| json + zstd | 81 B | 1.1 us | 1.2 us |
| msgpack | 49 B | 262 ns | 422 ns |
| msgpack + gzip | 74 B | 16.2 us | 1.2 us |
| msgpack + zstd | 62 B | 1.1 us | 610 ns |

**A product** with a description of 150 words:

| Codec + compressor | Size | Encode | Decode |
|---|---|---|---|
| json | 972 B | 1.3 us | 5.5 us |
| json + gzip | 346 B | 30.4 us | 12.1 us |
| json + zstd | 341 B | 10.2 us | 9.3 us |
| msgpack | 945 B | 512 ns | 732 ns |
| msgpack + gzip | 339 B | 30.7 us | 7.2 us |
| msgpack + zstd | 331 B | 10.2 us | 4.9 us |

**A list** of 500 products:

| Codec + compressor | Size | Encode | Decode |
|---|---|---|---|
| json | 108.2 KB | 204 us | 1.01 ms |
| json + gzip | 20.1 KB | 4.00 ms | 1.31 ms |
| json + zstd | 22.3 KB | 800 us | 1.16 ms |
| msgpack | 93.7 KB | 163 us | 285 us |
| msgpack + gzip | 19.5 KB | 4.49 ms | 573 us |
| msgpack + zstd | 22.6 KB | 712 us | 395 us |

**Recommended:**

- **Codec: `msgpack`.** It decodes 2 to 7 times faster than JSON at every size, and every hit
  decodes. Keep JSON where people or other services read the entries.
- **Compressor: none for small values, `zstd` for large ones.** A value of a few dozen bytes, such
  as the user, gets bigger and slower. From about 1 KB zstd stores 3 to 5 times less here, for
  under a millisecond even on the list. gzip stores about the same but encodes 3 to 15 times slower:
  take it only where a third-party dependency is not allowed.
- **One `Cache` compresses all its values**: for both small and large ones, use two caches, such as
  one with `Compressor: zstd.New()` for pages and lists.

# More

- [Examples](examples/): one runnable program per feature.
- [pkg.go.dev](https://pkg.go.dev/github.com/eugeneliukindev/gashe): the doc comments of every call.
- [Changelog](CHANGELOG.md), [glossary](docs/GLOSSARY.md), [contributing](CONTRIBUTING.md).

Under the [MIT license](LICENSE). The gopher in the logo is based on the Go gopher by
[Renee French](https://reneefrench.blogspot.com/), licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/), and was redrawn for gashe.
