# gashe

[![Go Reference](https://pkg.go.dev/badge/github.com/eugeneliukindev/gashe.svg)](https://pkg.go.dev/github.com/eugeneliukindev/gashe)

Caching for Go services that run as many processes. One `*gashe.Cache` stores results of expensive
calls, keeps answering when a dependency is slow or down, and guards dependencies with locks, rate
limits and circuit breakers. Over Redis all of it is shared by every pod.

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/backend/redis # Redis backend, on go-redis v9
go get github.com/eugeneliukindev/gashe/codec/msgpack # MessagePack codec, optional
```

Go 1.23 or newer. The core module has no third-party dependencies and no global state.

## Example

Four parts: a **backend** where entries live, **`gashe.Options`** for the whole cache, a
**`gashe.Policy`** for each kind of data, and the calls. `db`, `payments` and `clientIP` stand for
your own code.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct {
	ID   string
	Name string
}

var errNoUser = errors.New("user not found")

// A Policy says how one kind of data is cached. Define it once, next to the code that loads it.
var users = gashe.Policy[User]{
	Prefix:               "user:",          // stored under "user:<id>"
	TTL:                  10 * time.Minute, // fresh for 10 minutes
	Jitter:               0.1,              // spread expiry by up to 10%, so users do not expire together
	StaleWhileRevalidate: time.Minute,      // the minute after: answer the old user, reload it in the background
	StaleIfError:         time.Hour,        // the database is down: answer the old user for up to an hour
	LockOnMiss:           5 * time.Second,  // a cold key: one process loads it, the others wait for the result
	Negative: gashe.Negative{ // "no such user" is cached too, so unknown ids do not reach the database
		If:  func(err error) bool { return errors.Is(err, errNoUser) },
		Err: errNoUser,
		TTL: time.Minute,
	},
	Tags: []string{"users"}, // c.InvalidateTags(ctx, "users") drops every user at once
}

// GetOrLoadMany takes the plain options only.
var userList = gashe.Policy[User]{Prefix: "user:", TTL: 10 * time.Minute, Tags: []string{"users"}}

func main() {
	ctx := context.Background()

	// The backend is where entries live: memory for one process, Redis to share them between
	// processes: gasheredis.New(goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})).
	backend := memory.New(memory.Options{MaxBytes: 256 << 20})
	defer backend.Close()

	// Options say how the whole Cache behaves. Every field is optional.
	c := gashe.New(backend, gashe.Options{
		OnError:  func(ctx context.Context, err error) { slog.WarnContext(ctx, "cache", "error", err) },
		OnResult: func(ctx context.Context, key string, r gashe.Result) { /* count hits for metrics */ },
	})
	defer c.Shutdown(ctx) // let background reloads finish before the backend closes

	// One value: from the cache, or from the loader, which is then stored.
	u, err := gashe.GetOrLoad(ctx, c, "42", users, func(ctx context.Context) (User, error) {
		return db.User(ctx, "42")
	})
	fmt.Println(u, err)

	// Many values: one cache read, then one loader call for the missing ones.
	list, err := gashe.GetOrLoadMany(ctx, c, []string{"1", "2", "3"}, userList,
		func(ctx context.Context, missing []string) (map[string]User, error) {
			return db.Users(ctx, missing)
		})
	fmt.Println(list, err)

	// After a write: reload one user, or drop every user at once.
	if _, err := gashe.Reload(ctx, c, "42", users, func(ctx context.Context) (User, error) {
		return db.User(ctx, "42")
	}); err != nil {
		fmt.Println(err)
	}
	if err := c.InvalidateTags(ctx, "users"); err != nil {
		fmt.Println(err)
	}

	// Protecting dependencies: a rate limit, a circuit breaker, a lock.
	if err := c.RateLimit(ctx, "api:"+clientIP, gashe.PerMinute(100)); errors.Is(err, gashe.ErrRateLimited) {
		return // answer 429
	}
	if done, err := c.CircuitBreaker("payments", gashe.BreakerOptions{
		FailurePercent: 50, MinCalls: 20, Window: time.Minute, OpenFor: 30 * time.Second,
	}); err == nil { // gashe.ErrCircuitOpen while the payment API keeps failing
		done(payments.Charge(ctx))
	}
	if lease, ok, err := c.TryLock(ctx, "job:daily-report", 10*time.Minute); err == nil && ok {
		defer lease.Unlock(ctx)
		// only one process runs the daily report
	}
}
```

## Contents

- [Configuration](#configuration)
- [Backends](#backends): [memory](#memory), [Redis](#redis), [near cache](#near-cache-backend),
  [your own](#your-own-backend)
- [Keys and TTL](#keys-and-ttl)
- [Basic API](#basic-api)
- [Strategies](#strategies)
  - Caching: [cache-aside](#cache-aside), [stale-while-revalidate](#stale-while-revalidate),
    [stale-if-error](#stale-if-error), [negative caching](#negative-caching),
    [HTTP responses](#whole-http-responses)
  - Invalidation: [by tag](#invalidation-by-tag), [other ways](#other-ways-to-invalidate)
  - Load on the source: [stampede inside a process](#cache-stampede-inside-a-process),
    [stampede across processes](#cache-stampede-across-processes), [batch loading](#batch-loading),
    [TTL jitter](#ttl-jitter), [near cache](#near-cache)
  - Storage: [key prefixes](#key-prefixes), [large values](#large-values)
  - Protecting dependencies: [distributed lock](#distributed-lock), [rate limiting](#rate-limiting),
    [circuit breaker](#circuit-breaker)
  - Operations: [cache failure](#cache-failure), [monitoring](#monitoring),
    [graceful shutdown](#graceful-shutdown), [known limits](#known-limits)
- [Errors](#errors)
- [Testing your code](#testing-your-code)

## Configuration

Every option struct works empty; a field you leave out gets its default.

```go
mem := memory.New(memory.Options{
	MaxEntries:    10_000,      // 0 means 1000
	MaxBytes:      256 << 20,   // bytes of keys and values; 0 means no limit
	PurgeInterval: time.Minute, // how often expired entries are purged; 0 means every second
})
defer mem.Close()

c := gashe.New(mem, gashe.Options{
	Codec:            gashe.JSON{},          // the default
	OnError:          onError,               // errors the cache works around instead of returning
	OnResult:         onResult,              // how every GetOrLoad was answered, for metrics
	LockPollInterval: 20 * time.Millisecond, // how often Lock and LockOnMiss check; 0 means 100ms
})
defer c.Shutdown(ctx) // runs before mem.Close: waits for background reloads
```

Create one `Cache` at startup and share it; it is safe for concurrent use. It does not close the
backend: close it where you created it.

## Backends

### Memory

`backend/memory` is an LRU in process memory with a TTL per entry. Each process has its own; use
Redis to share data. Bytes are copied in and out, so changing a value after `Set` or `Get` never
changes the cache.

> [!WARNING]
> `MaxEntries` counts entries, not bytes: 10,000 entries of 1 MB are 10 GB. Set `MaxBytes` too,
> with headroom, since it does not count the overhead of Go. A value larger than `MaxBytes` is not
> stored: `Set` returns `memory.ErrTooLarge`.

### Redis

`backend/redis` is a separate module on [go-redis v9](https://github.com/redis/go-redis), so the
core stays dependency-free. You build and close the client; timeouts, pool, TLS, Sentinel and
Cluster are go-redis options. Valkey, Dragonfly and KeyDB pass the same tests.

```go
client := goredis.NewUniversalClient(&goredis.UniversalOptions{
	Addrs:        []string{"localhost:6379"},
	ReadTimeout:  100 * time.Millisecond, // a slow cache must not slow the service down
	WriteTimeout: 100 * time.Millisecond,
})
defer client.Close()
c := gashe.New(gasheredis.New(client), gashe.Options{})
```

- Every entry is a plain Redis string; [Strategies](#strategies) show what each one stores.
  Read them with `redis-cli`, but do not write `GetOrLoad` entries by hand: the format is internal.
  `DEL` is always safe.
- On a Cluster, multi-key calls go one key at a time, and `Keys`, `All` and `DeleteMatch` visit
  every master.
- Errors are wrapped as `gashe/redis: <op>: <error>`; a missing key is `gashe.ErrNotFound`.

> [!CAUTION]
> `Keys`, `All`, `DeleteMatch` and `redis-cli KEYS *` walk the whole database. On a large Redis
> prefer `InvalidateTags`, and look around with `redis-cli --scan --pattern 'user:*'`.

### Near cache backend

`backend/nearcache` puts a memory cache in front of Redis, see [Near cache](#near-cache).

### Your own backend

Implement `gashe.Backend`, which works with bytes only; the method comments in `backend.go` are the
contract. To change one behavior, embed `gashe.Backend` in a struct and override that method. Check
it with the suite the bundled backends pass:

```go
func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // a fresh backend for every subtest
	})
}
```

## Keys and TTL

The key that reaches the backend has up to three parts:

```go
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), "orders:prod:"), gashe.Options{}) // per service
users := gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"}                         // per kind
gashe.GetOrLoad(ctx, c, "42", users, load) // stored as "orders:prod:users:v2:42"
```

Prefixes are used as is, separator included. The library keeps its own keys under `gashe:tag:`,
`srl:`, `rate_limit:` and next to an entry as `<key>:load` and `<key>:refresh`.

In `GetOrLoad`, `TTL` is how long a result is fresh; the entry is kept `TTL` plus the longer of
`StaleWhileRevalidate` and `StaleIfError`. In the basic API a TTL of 0 means "never expires" and a
negative one returns `ErrInvalidTTL`.

> [!WARNING]
> Two call sites that read one key as different types make each other miss on every call. Give
> every type its own key or `Prefix`.

## Basic API

A map with TTL. `GetOrLoad` and the strategies below are built on the same entries.

```go
c.Set(ctx, "key", v, time.Hour)          // ttl 0 = no expiry; replaces the old TTL
c.SetIfAbsent(ctx, "key", v, time.Hour)  // (bool, error)
c.SetIfPresent(ctx, "key", v, time.Hour) // (bool, error)
c.SetMany(ctx, map[string]any{"a": 1}, time.Hour)

c.Get(ctx, "key", &dst)                 // gashe.ErrNotFound on a miss
values, err := c.GetMany(ctx, "a", "b") // values[i].Found(), values[i].Decode(&dst)
for key, err := range c.Keys(ctx, "user:*") {}
for v, err := range c.All(ctx, "user:*") {} // v.Key(), v.Decode(&dst)

c.Incr(ctx, "counter", 1, time.Minute) // ttl only when the counter is created
c.Exists(ctx, "key")
c.Delete(ctx, "key")
c.Expire(ctx, "key", time.Minute)
c.TTL(ctx, "key") // gashe.NoExpiry for no expiry
```

Patterns are Redis globs: `*`, `?`, `[abc]`, `[^a]`, `[a-z]`, `\` to escape. Values are anything
the codec can encode; with JSON, exported fields only. `GetOrLoad` returns the value as it reads
back from the cache, so a field that is lost shows up on the first call, not only on a hit.

## Strategies

Each section names a problem caches run into and the option that solves it. Most are fields of one
`gashe.Policy[V]` and can be combined. Over `memory` everything stays inside one process; over Redis
it is shared by every process that uses that Redis, unless a section says otherwise. "In Redis"
shows what the example stores, as `redis-cli` prints it. `loadUser`, `s.repo` and the like stand for
your own code.

| Problem | Solution |
|---|---|
| The same expensive call again and again | [`GetOrLoad`](#cache-aside) |
| Every request waits when a popular key expires | [`StaleWhileRevalidate`](#stale-while-revalidate) |
| The database is down and the cache is empty | [`StaleIfError`](#stale-if-error) |
| Lookups of missing ids always reach the database | [`Negative`](#negative-caching) |
| A heavy page is rendered on every request | [`httpcache`](#whole-http-responses) |
| One change makes many entries stale | [`Tags`, `InvalidateTags`](#invalidation-by-tag) |
| A cold key: a thousand identical queries | built in for one process, [`LockOnMiss`](#cache-stampede-across-processes) for all pods |
| N+1 queries on a list page | [`GetOrLoadMany`](#batch-loading) |
| Keys stored together expire together | [`Jitter`](#ttl-jitter) |
| A hot key costs a round trip on every read | [`nearcache`](#near-cache) |
| Work must run once at a time across pods | [`Lock`, `TryLock`](#distributed-lock) |
| A client calls far too often | [`RateLimit`](#rate-limiting) |
| A failing dependency drags the service down | [`CircuitBreaker`](#circuit-breaker) |

If the cache itself fails, `GetOrLoad` serves from the loader and reports the error to `OnError`.

### Caching

#### Cache-aside

The same expensive call is made again and again. `GetOrLoad` returns the stored result while it is
fresh; otherwise it calls your loader, stores the result for `TTL` and returns it. A loader error
is returned as is and never stored.

```go
var users = gashe.Policy[User]{TTL: time.Hour}

u, err := gashe.GetOrLoad(ctx, c, "user:42", users, func(ctx context.Context) (User, error) {
	return loadUser(ctx, 42) // runs only when the cache cannot answer
})
```

Use the `ctx` the loader receives, not the one around it: a background reload calls the loader
after the request has ended. Three more fields fit most cases:

```go
gashe.Policy[[]Item]{TTL: time.Minute, CacheIf: func(v []Item) bool { return len(v) > 0 }} // skip empty results
gashe.Policy[Token]{TTLFunc: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }}
gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"} // stores "users:v2:42"; change it when User changes
```

In Redis, one string per key: `at` is when the result stops being fresh, `v` is the result.

```console
$ redis-cli GET user:42
"{\"at\":\"2026-11-02T16:21:48.62510671+03:00\",\"v\":{\"ID\":42,\"Name\":\"Ann\",\"Email\":\"ann@example.com\"}}"
$ redis-cli TTL user:42
(integer) 3600
```

#### Stale-while-revalidate

A popular key expires, and every request waits for the slow loader. With `StaleWhileRevalidate`
(RFC 5861), for this long after `TTL` the stale result is answered at once while one caller
reloads it in the background; with Redis, one caller across all processes.

```go
var feeds = gashe.Policy[[]Item]{TTL: 5 * time.Minute, StaleWhileRevalidate: 30 * time.Minute}
```

In Redis, the entry lives `TTL + StaleWhileRevalidate`; during a reload the refresh lock holds the
id of the process that reloads, for at most a minute.

```console
$ redis-cli TTL feed:u1
(integer) 2100
$ redis-cli GET feed:u1:refresh
"f4fefbc6731d4abccff956cae8ca012f"
```

> [!CAUTION]
> The background reload runs after the request has ended: its `ctx` keeps the values of the
> request but not its cancellation, and gets a one-minute deadline. Do not use request-scoped
> resources, such as an open transaction, inside the loader.

#### Stale-if-error

The database or an API is down, and expired entries leave nothing to answer with. With
`StaleIfError` (RFC 5861), for this long after `TTL` a stale result answers a loader error; the
error goes to `OnError`. `StaleIf` picks which errors count.

```go
var rates = gashe.Policy[Rates]{
	TTL:          time.Minute,
	StaleIfError: 6 * time.Hour,
	StaleIf:      func(err error) bool { return errors.Is(err, ErrAPIDown) }, // nil = any error
}
```

In Redis, the entry is fresh for a minute but kept `TTL + StaleIfError`:

```console
$ redis-cli TTL rates:usd
(integer) 21660
```

> [!WARNING]
> An answer can be up to `TTL + StaleIfError` old. Do not use it where a stale answer is wrong
> rather than late: permissions, balances, prices at checkout.

#### Negative caching

Requests for data that does not exist (a scraper walking ids) are never cached, so all of them
reach the database. `Negative` caches "not found" for a short while and returns the error you chose
while it is cached.

```go
var users = gashe.Policy[User]{
	TTL: time.Hour,
	Negative: gashe.Negative{
		If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) }, // which errors mean "no such data"
		Err: sql.ErrNoRows,                                                 // returned while cached
		TTL: 30 * time.Second,
	},
}
```

In Redis, an ordinary entry with `"neg":true`:

```console
$ redis-cli GET user:999
"{\"at\":\"2026-11-02T15:22:19.930721677+03:00\",\"v\":{\"ID\":0,\"Name\":\"\",\"Email\":\"\"},\"neg\":true}"
```

> [!WARNING]
> Data created after a negative answer stays "not found" until `Negative.TTL` ends. Keep it short,
> or `Delete` the key when you create the data.

#### Whole HTTP responses

A page or a JSON response is the same for everyone but costly to render on every request.
`httpcache` caches whole responses of a handler: a hit sends the stored status, headers and body
without calling the handler. It runs on `GetOrLoad`, so every `Policy` field works for pages: a
5xx with `StaleIfError` gets the last good page, `LockOnMiss` renders a cold page once for all pods.

```go
cached := httpcache.New(c, httpcache.Options{Policy: gashe.Policy[httpcache.Response]{
	Prefix:       "http:",
	TTL:          time.Minute,
	StaleIfError: time.Hour,
	Tags:         []string{"catalog"}, // c.InvalidateTags(ctx, "catalog") after an edit
}})
mux.Handle("GET /products/", cached(http.HandlerFunc(listProducts)))
```

Only `GET` and `HEAD` are cached, keyed by host, path and query. Responses get an `ETag`, and a
matching `If-None-Match` gets `304` without a body; `X-Cache` says `hit` or `miss`. Request
`Cache-Control` is ignored, so clients cannot make the handler run on every request.

> [!CAUTION]
> A response must be the same for everyone who gets its key. A response with `Set-Cookie`, `Vary`
> or `Cache-Control: private` is never stored, and requests with `Authorization` skip the cache.
> If a page depends on a cookie, set `Key` and put the user in it, or do not wrap that route.

### Invalidation

#### Invalidation by tag

One change makes many entries stale, and deleting them by pattern scans the whole of Redis. Give
entries `Tags`; `InvalidateTags` makes every entry with the tag a miss with one write per tag.

```go
profile := gashe.Policy[Profile]{TTL: time.Hour, Tags: []string{"user:42"}}
orders := gashe.Policy[[]Order]{TTL: time.Hour, Tags: []string{"user:42", "orders"}}

_, err = db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = 42", name)
err = c.InvalidateTags(ctx, "user:42") // after the write, not before
```

In Redis, a tag is a counter that appears on its first invalidation; `g` in an entry holds the
counters it was stored with, so entries with an old `g` are misses and expire by TTL:

```console
$ redis-cli GET orders:42
"{\"at\":\"2026-11-02T16:21:50.147958692+03:00\",\"v\":[{\"ID\":7}],\"g\":[0,0]}"
$ redis-cli GET gashe:tag:user:42
"1"
```

> [!NOTE]
> Tag counters never expire: every tag name is one small key forever. Tags per user are fine;
> tags per request are not.

#### Other ways to invalidate

```go
gashe.Reload(ctx, c, "user:42", users, loadUser) // load and store a new result now
c.Delete(ctx, "user:42")
c.DeleteMatch(ctx, "user:*")                      // scans the whole backend: avoid on a large Redis
```

To drop everything of one kind after a code change, change its `Prefix`. Every key then misses at
once: expect the load of a cold cache after such a deploy.

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
var stats = gashe.Policy[Stats]{TTL: 10 * time.Minute, LockOnMiss: time.Minute}
```

In Redis, while one pod loads:

```console
$ redis-cli GET stats:2026-11-02:load
"64a228b2be4e789acbfa2b626f87c114"
$ redis-cli TTL stats:2026-11-02:load
(integer) 60
```

> [!WARNING]
> Make `LockOnMiss` longer than the slowest load, or a second pod starts loading when the lock
> expires. Waiting calls wait as long as their `ctx` allows.

#### Batch loading

A page shows 50 products: 50 cache round trips and, on a cold cache, 50 queries. `GetOrLoadMany`
reads all keys in one call and calls the loader once with the missing ones; keys the loader leaves
out are not stored. It shares entries with `GetOrLoad` and supports `TTL`, `TTLFunc`, `Jitter`,
`Tags`, `CacheIf` and `Prefix`; the other options return `ErrInvalidOptions`.

```go
var catalog = gashe.Policy[Product]{TTL: time.Hour, Prefix: "product:"}

products, err := gashe.GetOrLoadMany(ctx, c, ids, catalog,
	func(ctx context.Context, missing []string) (map[string]Product, error) {
		return s.repo.ProductsByID(ctx, missing) // SELECT ... WHERE id = ANY($1)
	})
```

Concurrent calls are not shared: two requests for the same cold list both call the loader.

#### TTL jitter

Keys stored together (after a deploy or a warm-up) expire together, and the database gets all of
their loads at once (cache avalanche). `Jitter` spreads every TTL randomly by up to this share.

```go
var products = gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.2} // 48 to 72 minutes
```

In Redis, products stored at the same moment:

```console
$ redis-cli TTL product:1
(integer) 3009
$ redis-cli TTL product:2
(integer) 4183
```

#### Near cache

A hot key is read thousands of times a second, each read a network round trip to one Redis node.
`nearcache` keeps a copy in process memory for `LocalTTL`; writes and deletes go to Redis and drop
the local copy. Locks, counters and TTLs always use Redis.

With a `Bus`, a pod that changes a key tells the others, and they drop their copies at once instead
of after `LocalTTL`. Only the key name is sent; the next read takes the new value from Redis.
`InvalidateTags` becomes instant on every pod too.

```go
local := memory.New(memory.Options{MaxBytes: 64 << 20})
defer local.Close()
near := nearcache.New(local, gasheredis.New(client), nearcache.Options{
	LocalTTL: time.Minute,
	Bus:      gasheredis.NewBus(client, "orders:invalidate"), // one channel per service
})
defer near.Close() // stops listening to the bus
c := gashe.New(near, gashe.Options{})
```

> [!WARNING]
> Every pod keeps its own copy, so with 50 pods the local cache takes RAM 50 times: bound it with
> `MaxBytes`. The bus sees only changes made through gashe; a key changed by `redis-cli` or another
> service stays stale up to `LocalTTL`. While a pod is disconnected from Redis it may miss
> messages; it drops all its copies when it reconnects.

### Storage

#### Key prefixes

Several services or environments share one Redis and their keys collide. `WithPrefix` keeps every
key of a backend under a prefix; inside the service keys stay as they are.

```go
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), "orders:staging:"), gashe.Options{})
```

```console
$ redis-cli --scan
orders:staging:order:1
```

#### Large values

Big values take memory in Redis and network on every read. `Gzip` compresses what another codec
encodes; `codec/msgpack` is a smaller binary format; `Raw` stores `[]byte` and `string` as they are
in the basic API.

```go
c := gashe.New(backend, gashe.Options{Codec: gashe.Gzip{}}) // a 320 KB page takes 2 KB in Redis
```

> [!CAUTION]
> Changing the codec, like changing `Prefix`, makes every stored entry a miss at once. `Incr` does
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
defer lease.Unlock(ctx) // releases it only if it is still yours
for more() {
	work()
	if err := lease.Extend(ctx, time.Minute); err != nil {
		return err // gashe.ErrLockLost: stop, another pod may have started
	}
}
```

In Redis, the random id of the holder:

```console
$ redis-cli GET job:reindex
"621708739092f7d5ef73f2358d22a804"
```

> [!CAUTION]
> Locks are advisory: a holder paused longer than the TTL (GC, a frozen VM) can overlap with the
> next one. Make writes that guard money idempotent too.

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

In Redis, a sliding window keeps a counter per span of `Per`; a fixed window is one counter, whose
TTL the first call over the limit sets to `BanFor`:

```console
$ redis-cli GET srl:api-key:k1:29893701
"3"
$ redis-cli TTL rate_limit:login:1.2.3.4
(integer) 900
```

`SlidingWindow` is an estimate and can let slightly more through under bursts; `FixedWindow` can
let up to twice `Calls` through around the border of two windows.

#### Circuit breaker

A dependency starts failing, every request still waits for it, and the failure spreads. The breaker
counts calls of the last `Window`; once `FailurePercent` of them fail, it refuses calls with
`ErrCircuitOpen` for `OpenFor`, then lets `HalfOpenCalls` test calls through. It lives in process
memory: each pod judges the dependency by its own calls, and Redis holds nothing.

```go
var payments = gashe.BreakerOptions{FailurePercent: 50, MinCalls: 10, Window: 30 * time.Second,
	OpenFor: 15 * time.Second, HalfOpenCalls: 1}

done, err := c.CircuitBreaker("payments-api", payments)
if err != nil {
	return ErrPaymentsUnavailable // answer at once instead of waiting for a timeout
}
err = provider.Charge(ctx, p)
done(err) // report once: nil or the error
```

> [!WARNING]
> The state of every key is kept for the life of the `Cache`: use a few fixed keys, not user ids
> or URLs.

### Operations

#### Cache failure

Built in: if the backend fails, `GetOrLoad` returns the loader result without storing it,
`RateLimit` lets the call through, and the error goes to `OnError`. Bound how long a slow Redis can
hold a request with go-redis `ReadTimeout` and `WriteTimeout`.

> [!WARNING]
> With Redis down, `LockOnMiss` no longer holds other pods back, and every pod sends its misses to
> the database. Put a `CircuitBreaker` in front of the database for that case.

#### Monitoring

`OnResult` receives how every `GetOrLoad` was answered: hit, miss, stale-while-revalidate,
stale-if-error or negative-hit. It sees the calls of this process; sum the metrics of all pods.

```go
c := gashe.New(backend, gashe.Options{
	OnResult: func(ctx context.Context, key string, r gashe.Result) { results.WithLabelValues(r.String()).Inc() },
	OnError:  func(ctx context.Context, err error) { slog.WarnContext(ctx, "cache degraded", "error", err) },
})
```

Both run synchronously on every call: keep them fast.

#### Graceful shutdown

A background reload may still run when the service stops and write to a closed backend.
`Shutdown` stops starting reloads and waits for the running ones; call it before closing Redis.

```go
_ = srv.Shutdown(ctx) // the HTTP server first
_ = c.Shutdown(ctx)
_ = client.Close()
```

#### Known limits

- **Cache-aside race.** A read can load an old value, a write can update the database and delete
  the key, and then the read stores its old value until its TTL ends. Keep TTLs of data that
  changes often short, or invalidate its tag after the write.
- **Locks are advisory**, see [distributed lock](#distributed-lock).
- **Clocks.** Freshness compares the clock of the reading process with the time the writer stored.
  Keep clocks in sync (NTP).

## Errors

Check them with `errors.Is`.

| Error | When |
|---|---|
| `ErrNotFound` | `Get`, `TTL`, `Value.Decode` on a miss |
| `ErrInvalidTTL` | a negative TTL, or a non-positive one where a lock or `Expire` needs it |
| `ErrInvalidOptions` | a `Policy`, `Limit` or `BreakerOptions` that cannot work |
| `ErrWrongType` | `Incr` on a value that is not an integer |
| `ErrLockLost` | `Lease.Extend` of a lock that expired |
| `ErrRateLimited` | `RateLimit` refuses a call |
| `ErrCircuitOpen` | `CircuitBreaker` refuses a call |

Loader errors are returned unchanged. `GetOrLoad` and `RateLimit` never return backend errors:
they work around them and send them to `OnError`. `Lock`, `TryLock`, `Extend` and
`InvalidateTags` do return them.

## Testing your code

Use the memory backend with the purge off, so no goroutine outlives the test:

```go
func newCache(t *testing.T) *gashe.Cache {
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	return gashe.New(mem, gashe.Options{})
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

- [API reference](https://pkg.go.dev/github.com/eugeneliukindev/gashe) on pkg.go.dev, and
  [REFERENCE.md](REFERENCE.md) in the repository.
- [Changelog](CHANGELOG.md) of every release.
- [Glossary](GLOSSARY.md) of the terms used here, and [Contributing](CONTRIBUTING.md).

Under the [MIT license](LICENSE).
