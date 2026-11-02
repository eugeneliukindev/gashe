# gashe

[![Go Reference](https://pkg.go.dev/badge/github.com/eugeneliukindev/gashe.svg)](https://pkg.go.dev/github.com/eugeneliukindev/gashe)

English | [Russian](README.ru.md)

Caching for Go services that run as many processes: store results of expensive calls, keep
answering when a dependency is slow or down, and guard dependencies with locks, rate limits and
circuit breakers, all around one `*gashe.Cache`.

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/backend/redis # Redis backend, depends on go-redis v9
go get github.com/eugeneliukindev/gashe/codec/msgpack # MessagePack codec, optional
```

Requires Go 1.23 or newer.

## Why

A cache is more than `Get` and `Set`. Production services hit the same handful of problems: a cold
key sends a hundred identical queries to the database at once, a popular key expires and every
request waits for the reload, the database goes down and the cache has nothing to answer with,
lookups of ids that do not exist go to the database every time, thousands of keys expire in the
same second. gashe solves each of them with one option or one method, and keeps working across
processes that share one Redis.

## Features

- Typed caching: `gashe.GetOrLoad(ctx, c, key, opts, load)` returns your type.
- Cache stampede protection inside a process (automatic) and across processes (`LockOnMiss`).
- Stale-while-revalidate (`StaleWhileRevalidate`) and stale-if-error (`StaleIfError`), in one entry.
- Negative caching for "not found" answers, TTL jitter against mass expiry.
- Batch loading of lists (`GetOrLoadMany`): one cache read and one loader call for all missing keys.
- Tag invalidation in O(1) (`Tags`, `InvalidateTags`), without scanning the backend.
- Distributed locks with extension, sliding and fixed rate limits, an in-process circuit breaker.
- Backends: an in-memory LRU with TTL, Redis (single node, Sentinel or Cluster) in a separate
  module, a two-level near cache (memory in front of Redis), key prefixes for a shared Redis.
- Pluggable `Codec`: JSON by default, gzip for large values, raw bytes, MessagePack in a separate
  module.
- Hooks for errors the cache works around (`OnError`) and for hit and miss counts (`OnResult`).
- When the cache itself fails, your code keeps working.
- No global state and no third-party dependencies in the core module; Go 1.23 to the newest release.

## Table of Contents

- [API reference](REFERENCE.md), a separate file
- [Configuration](#configuration)
- [Available Backends](#available-backends)
- [Across processes](#across-processes)
- [Basic API](#basic-api)
- [Keys](#keys)
- [TTL](#ttl)
- [What can be cached](#what-can-be-cached)
- [Strategies](#strategies)
  - [Caching](#caching)
    - [Caching expensive calls (cache-aside)](#caching-expensive-calls-cache-aside)
    - [Latency spikes on expiry (stale-while-revalidate)](#latency-spikes-on-expiry-stale-while-revalidate)
    - [Dependency outage (stale-if-error)](#dependency-outage-stale-if-error)
    - [Cache penetration (negative caching)](#cache-penetration-negative-caching)
  - [Load on the source](#load-on-the-source)
    - [Cache stampede inside a process](#cache-stampede-inside-a-process)
    - [Cache stampede across processes](#cache-stampede-across-processes)
    - [N+1 queries on lists (batch loading)](#n1-queries-on-lists-batch-loading)
    - [Cache avalanche (TTL jitter)](#cache-avalanche-ttl-jitter)
    - [Hot keys and network round trips (near cache)](#hot-keys-and-network-round-trips-near-cache)
  - [Storage](#storage)
    - [Key collisions in a shared Redis](#key-collisions-in-a-shared-redis)
    - [Large values](#large-values)
  - [Protecting dependencies](#protecting-dependencies)
    - [Mutual exclusion across processes (distributed lock)](#mutual-exclusion-across-processes-distributed-lock)
    - [Overload and abuse (rate limiting)](#overload-and-abuse-rate-limiting)
    - [Cascading failures (circuit breaker)](#cascading-failures-circuit-breaker)
  - [Operations](#operations)
    - [Cache failure](#cache-failure)
    - [Hit ratio and monitoring](#hit-ratio-and-monitoring)
    - [Graceful shutdown](#graceful-shutdown)
  - [Known limits](#known-limits)
- [Cache Invalidation](#cache-invalidation)
  - [Invalidating by tag](#invalidating-by-tag)
- [Errors](#errors)
- [Testing your code](#testing-your-code)
- [Roadmap](#roadmap)
- [Development](#development)

## Usage Example

The library has four parts: a **backend** where entries live, **`gashe.Options`** for the whole
cache, a **`gashe.Policy`** for each kind of data, and the calls. The example below uses all of
them; `db`, `payments` and `clientIP` stand for your own code.

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

| Part | What it is | Where you set it |
|---|---|---|
| Backend: `memory.New`, `gasheredis.New`, `nearcache.New` | where entries live, and so which processes share them | once at startup |
| `gashe.Options` | error and metrics hooks, codec, lock polling | once, in `gashe.New` |
| `gashe.Policy[V]` | TTL and the strategies for one kind of data | a package variable per kind, passed to every call |
| `gashe.GetOrLoad`, `GetOrLoadMany`, `Reload` | caching | where you load data |
| `c.InvalidateTags`, `c.Delete` | invalidation | after writes |
| `c.Lock`, `c.TryLock`, `c.RateLimit`, `c.CircuitBreaker` | protecting dependencies | around the work they guard |
| `c.Get`, `c.Set`, ... | the basic API, like a map with TTL | anywhere |
| `c.Shutdown` | waits for background reloads | at the end of `main` |

Every option is explained in [Configuration](#configuration) and [Strategies](#strategies). What every
function, method and field takes and returns is in the [API reference](REFERENCE.md).

## Configuration

Everything is configured with option structs. A field you leave out gets its default, so an
empty struct always works.

```go
mem := memory.New(memory.Options{
	MaxEntries:    10_000,      // keep at most this many entries; 0 means 1000
	MaxBytes:      256 << 20,   // and at most this many bytes of keys and values; 0 means no limit
	PurgeInterval: time.Minute, // how often to throw out expired entries; 0 means every second
})
defer mem.Close()

c := gashe.New(mem, gashe.Options{
	Codec:            gashe.JSON{},          // how values become bytes; leave it out for JSON
	OnError:          onError,               // errors the cache works around
	OnResult:         onResult,              // how every GetOrLoad was answered
	LockPollInterval: 20 * time.Millisecond, // how often a waiting Lock checks; 0 means 100ms
})
```

| `gashe.Options` field | What it is for |
|---|---|
| `Codec` | Turns your values into bytes for the backend and back. JSON unless you set another. |
| `OnError` | `GetOrLoad` and `RateLimit` never fail just because the cache is broken: they carry on without it. The cache error is not lost, it goes to this function. Without it such errors are dropped. |
| `OnResult` | Receives `gashe.ResultHit`, `ResultMiss`, `ResultStaleWhileRevalidate`, `ResultStaleIfError` or `ResultNegativeHit` for every `GetOrLoad` call, with the stored key. Use it for hit ratio metrics. |
| `LockPollInterval` | How often `Lock` and `LockOnMiss` check whether a busy lock is free. Lower it for locks held for milliseconds, raise it for long jobs to load Redis less. 0 means 100ms. |

Create one `Cache` at startup and share it; it is safe for concurrent use. It does not close the
backend: close the backend where you created it, after `c.Shutdown` (see
[Graceful shutdown](#graceful-shutdown)).

## Available Backends

### In-memory

`github.com/eugeneliukindev/gashe/backend/memory` keeps entries in process memory: a fixed-size LRU where
every entry may have its own TTL. Expired entries are never returned; they are removed when they are
read and by a background purge every `PurgeInterval`. Stored bytes are copied in and out, so
changing a value after `Set` or `Get` never changes the cache. Each process has its own memory
cache; to share data between processes use Redis.

Patterns in `Keys`, `All` and `DeleteMatch` use Redis glob syntax: `*`, `?`, `[abc]`, `[^a]`,
`[a-z]`, and `\` to escape.

> [!WARNING]
> `MaxEntries` alone counts entries, not bytes: 10,000 entries of 1 MB each is 10 GB of RAM. Set
> `MaxBytes` too. It counts only the bytes of keys and values, not the overhead of Go, so leave
> headroom. A value larger than `MaxBytes` is not stored: `Set` returns `memory.ErrTooLarge`, and
> `GetOrLoad` reports it to `OnError`. Every process keeps its own copy, and everything is lost on
> restart.

### Redis

`github.com/eugeneliukindev/gashe/backend/redis` is a separate module on top of
[go-redis v9](https://github.com/redis/go-redis), so the core module stays dependency-free. You build
the go-redis client yourself and close it yourself: the backend does not own it. Timeouts, pool
size, TLS, Sentinel and Cluster are go-redis options.

```go
import (
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
c := gashe.New(gasheredis.New(client), gashe.Options{})
```

Notes:

- Redis-compatible servers work through the same module: the integration tests pass on Redis 7.0,
  Valkey 8, Dragonfly and KeyDB.
- On a Cluster, `Keys`, `All` and `DeleteMatch` visit every master, and multi-key operations are
  sent one command per key, so keys may live in different slots.
- `Keys` and `All` may yield a key more than once, as Redis `SCAN` does.
- Redis errors are wrapped once, as `gashe/redis: <op>: <error>`. A missing key becomes
  `gashe.ErrNotFound`; a `WRONGTYPE` reply or a non-integer `Incr` becomes `gashe.ErrWrongType`.

> [!CAUTION]
> `Keys`, `All` and `DeleteMatch` walk the whole database with `SCAN`, on a Cluster every master.
> On a large Redis this takes long and loads it; prefer `InvalidateTags` for invalidation.

### Two-level (near cache)

`github.com/eugeneliukindev/gashe/backend/nearcache` puts a small memory cache in front of a shared backend.
See [Hot keys and network round trips](#hot-keys-and-network-round-trips-near-cache).

### Your own backend

Implement `gashe.Backend`. It works with bytes only: `Cache` encodes values before they reach the
backend. Read the method comments in `backend.go` for the exact contract: a `ttl` of 0 means no
expiry, a miss is `gashe.ErrNotFound`, and every method must be safe for concurrent use. To change
how a backend behaves, wrap it: embed `gashe.Backend` in a struct and override the methods you need.

Check your backend against the same contract suite the bundled backends pass:

```go
func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // a fresh, empty backend for every subtest
	})
}
```

## Across processes

What is shared is decided by the backend, not by the method. Below, **all processes** means every
process that uses the same Redis (or another shared backend), and **this process** means only the
process where the call runs. Strategies in [Strategies](#strategies) have the same **Scope** line.

| Feature | With Redis | With `memory` |
|---|---|---|
| Results of `GetOrLoad`, `GetOrLoadMany`, `Get`, `Set` | all processes | this process |
| Negative answers (`Negative`) | all processes | this process |
| The one background reload of `StaleWhileRevalidate` | one for all processes | one per process |
| `LockOnMiss`: one loader for a cold key | one for all processes | one per process |
| `Lock`, `TryLock`, `Lease.Extend` | all processes | this process |
| `RateLimit` | one limit for all processes | a separate limit in each process |
| `Tags`, `InvalidateTags` | all processes; through `nearcache` within `LocalTTL` | this process |
| Sharing of concurrent `GetOrLoad` calls | this process | this process |
| `CircuitBreaker` | this process | this process |
| `OnResult`, `OnError`, `Shutdown` | this process | this process |
| The local level of `nearcache` | this process | - |

## Basic API

```go
c.Set(ctx, "key", v, time.Hour)             // error; ttl 0 = no expiry, replaces the old TTL
c.SetIfAbsent(ctx, "key", v, time.Hour)     // (bool, error); only if the key is not there
c.SetIfPresent(ctx, "key", v, time.Hour)    // (bool, error); only if the key is there
c.SetMany(ctx, map[string]any{"a": 1}, ttl) // error

c.Get(ctx, "key", &dst)                     // error; gashe.ErrNotFound on a miss
values, err := c.GetMany(ctx, "a", "b")     // []gashe.Value, in key order
values[0].Found()                           // bool
values[0].Decode(&dst)                      // error; gashe.ErrNotFound if not found

for key, err := range c.Keys(ctx, "user:*") {} // every matching key
for v, err := range c.All(ctx, "user:*") {}    // every matching entry: v.Key(), v.Decode(&dst)

c.Incr(ctx, "counter", 1, time.Minute)      // (int64, error); ttl only when the key is created
c.Exists(ctx, "key")                        // (bool, error)
c.Delete(ctx, "key")                        // (bool, error)
c.DeleteMany(ctx, "a", "b")                 // error
c.DeleteMatch(ctx, "user:*")                // error
c.Expire(ctx, "key", time.Minute)           // (bool, error); ttl must be positive
c.TTL(ctx, "key")                           // (time.Duration, error); gashe.NoExpiry if none
c.Ping(ctx)                                 // error

c.Backend()                                 // gashe.Backend, for raw bytes
```

An integer written with `Incr` reads back with `Get` into an `int`, and a JSON number written with
`Set` can be incremented with `Incr`.

## Keys

Keys are strings you build in code from the call arguments:

```go
gashe.GetOrLoad(ctx, c, fmt.Sprintf("user:%d:lang:%s", id, lang), users, load)
gashe.GetOrLoad(ctx, c, fmt.Sprintf("token:%x", sha256.Sum256([]byte(token))), tokens, load)
```

The key that reaches Redis is built from up to three parts, outside in:

| Part | Set by | Use it for | Example |
|---|---|---|---|
| backend prefix | `gashe.WithPrefix(backend, "orders:prod:")` | several services or environments in one Redis | `orders:prod:` |
| policy prefix | `Policy.Prefix` | one kind of data and its version | `users:v2:` |
| key | the `key` argument | one item | `42` |

```go
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), "orders:prod:"), gashe.Options{})
users := gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"}
gashe.GetOrLoad(ctx, c, "42", users, load) // stored as "orders:prod:users:v2:42"
```

Both prefixes are used as is, with no separator added. Rate limits store under `srl:` and
`rate_limit:`, tag generations under `gashe:tag:`, so they never collide with your entries.
Different value types need different keys: reading a key as another type is a decode failure, which
`GetOrLoad` treats as a miss and reloads.

Service keys next to an entry: `<key>:refresh` (background reload lock, at most a minute) and
`<key>:load` (`LockOnMiss`).

> [!WARNING]
> Two call sites that use the same key with different types make each other miss on every call:
> each reload overwrites the other type. Give every type its own key or `Prefix`.

## TTL

TTLs are plain `time.Duration` values.

- In `GetOrLoad`, `TTL` is how long a result is fresh, and the entry is kept for `TTL` plus the
  longer of `StaleWhileRevalidate` and `StaleIfError`. `TTLFunc` picks the TTL per result, and `Jitter`
  spreads it.
- In the basic API (`Set`, `SetIfAbsent`, `Incr`, ...) a TTL of 0 means "never expires", and a
  negative one returns `gashe.ErrInvalidTTL`. `Set` always replaces the old TTL; `Incr` uses its TTL
  only when it creates the counter.

## What can be cached

Anything the codec can encode. With the default `gashe.JSON` codec that means values that
`encoding/json` can marshal and unmarshal back into the same type: exported struct fields, maps
with string keys, slices, numbers, strings and booleans. Unexported fields are not stored, and
`GetOrLoad` returns the value as it reads back from the cache, so you see that right away instead
of only on a hit.

To store other types, implement `gashe.Codec` or give your type `MarshalJSON` and `UnmarshalJSON`.

## Strategies

Each section names a problem caches run into, says how gashe solves it, and shows the
configuration. Most of them are options of one `gashe.Policy[V]` and can be combined.

### Caching

#### Caching expensive calls (cache-aside)

**Problem.** The same expensive call (a database query, an API request) is made again and again
with the same arguments.

**Solution.** `gashe.GetOrLoad` returns the stored result while it is fresh; otherwise it calls your
function (the **loader**), stores its result for `TTL` and returns it. An error from the loader is
returned as is and never stored.

```go
var users = gashe.Policy[User]{TTL: time.Hour} // the rules for caching users; define it once

u, err := gashe.GetOrLoad(ctx, c, "user:42", users, func(ctx context.Context) (User, error) {
	return db.LoadUser(ctx, 42) // runs only when the cache cannot answer
})
```

`GetOrLoad` is a function rather than a method of `c` because Go methods cannot have type
parameters before Go 1.27. Options that cannot work return `gashe.ErrInvalidOptions`.

Three optional fields of `Policy` fit most cases; the next sections add the rest.

- **`CacheIf`** - store a result only if it is worth it. Without it, every successful result is
  stored.

  ```go
  var search = gashe.Policy[[]Item]{
  	TTL:     time.Minute,
  	CacheIf: func(items []Item) bool { return len(items) > 0 }, // do not cache empty results
  }
  ```

  `gashe.NotNil` is a ready rule for pointers, slices and maps:
  `gashe.Policy[*User]{TTL: time.Hour, CacheIf: gashe.NotNil[*User]}`.

- **`TTLFunc`** - take the TTL from the result itself, when the result knows how long it is valid.
  A negative TTL means "do not store".

  ```go
  // An access token is cached until a minute before it expires.
  var tokens = gashe.Policy[Token]{
  	TTLFunc: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute },
  }
  ```

- **`Prefix`** - a string of your choice put in front of every key of this policy, as is (write the
  separator yourself). Use it to version cached data: when `User` gets new fields, change
  `"users:v1:"` to `"users:v2:"`, and the old entries are no longer read into the new type. To keep
  several services apart in one Redis, use `gashe.WithPrefix` instead, see
  [Keys](#keys).

  ```go
  var users = gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"} // stores "users:v2:42"
  u, err := gashe.GetOrLoad(ctx, c, "42", users, loadUser)
  ```

**Scope.** Stored results are seen by every process that uses the same Redis; with `memory`, by this
process only.

**Example.** A user from PostgreSQL, cached for an hour.

```go
var users = gashe.Policy[User]{TTL: time.Hour}

func (s *Service) User(ctx context.Context, id int64) (User, error) {
	return gashe.GetOrLoad(ctx, s.cache, fmt.Sprint("user:", id), users, func(ctx context.Context) (User, error) {
		var u User
		err := s.db.QueryRowContext(ctx, "SELECT id, name, email FROM users WHERE id = $1", id).
			Scan(&u.ID, &u.Name, &u.Email)
		return u, err
	})
}
```

#### Latency spikes on expiry (stale-while-revalidate)

**Problem.** A popular key expires; until the slow loader finishes, every request for it waits.

**Solution.** `StaleWhileRevalidate`, named after the `Cache-Control` directive of RFC 5861: for this
long after `TTL` the stale result is answered at once, while
exactly one caller across all processes reloads it in the background. Nobody waits for the loader.
A failed reload keeps the stale result and goes to `OnError`; a reload that hangs is stopped after a
minute.

```go
// Fresh for 10 minutes. In the next 2 minutes callers get the stale result immediately and one of
// them reloads it in the background.
var feed = gashe.Policy[Feed]{TTL: 10 * time.Minute, StaleWhileRevalidate: 2 * time.Minute}
```

With `TTL: 0` and only `StaleWhileRevalidate`, every call answers at once and triggers a background
reload; a key nobody reads within the window expires.

**Scope.** The stale answer comes from the stored entry, so it follows the backend. The background
reload runs once for all processes with Redis, and once per process with `memory`.

**Example.** A personal feed from a slow recommendation service: once warm, nobody waits for it again.

```go
var feeds = gashe.Policy[[]Item]{TTL: 5 * time.Minute, StaleWhileRevalidate: 30 * time.Minute}

func (s *Service) Feed(ctx context.Context, userID string) ([]Item, error) {
	return gashe.GetOrLoad(ctx, s.cache, "feed:"+userID, feeds, func(ctx context.Context) ([]Item, error) {
		return s.recommender.Feed(ctx, userID) // a 300ms call, now made in the background
	})
}
```

> [!CAUTION]
> The background reload runs after the request that started it has ended. Its `ctx` keeps the
> values of the request but not its cancellation, and gets a one-minute deadline. Do not use
> resources that live only as long as the request inside the loader, such as an open database
> transaction.

#### Dependency outage (stale-if-error)

**Problem.** The database or an API is down, and the cache has nothing to answer with once entries
expire.

**Solution.** `StaleIfError`, the other directive of RFC 5861: for this long after `TTL` a stale result
answers a loader error. The loader still runs first; only if it fails you get the stale result and a
nil error, and the loader
error goes to `OnError`. `StaleIf` limits which errors are covered.

```go
var rates = gashe.Policy[Rates]{
	TTL:            5 * time.Minute,
	StaleIfError: time.Hour,
	StaleIf:     func(err error) bool { return errors.Is(err, ErrAPIDown) }, // nil = any error
}
```

With `TTL: 0` and only `StaleIfError`, the loader runs on every call, and the last good result
answers its errors (a failover cache).

**Scope.** The stale answer comes from the stored entry: every process can answer with it with
Redis, only this process with `memory`.

**Example.** Exchange rates from an external HTTP API: if the provider is down, the last rates are served for up to six hours.

```go
var rates = gashe.Policy[map[string]float64]{TTL: time.Minute, StaleIfError: 6 * time.Hour}

func (s *Service) Rates(ctx context.Context) (map[string]float64, error) {
	return gashe.GetOrLoad(ctx, s.cache, "rates:usd", rates, func(ctx context.Context) (map[string]float64, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://rates.example.com/usd", nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, err // the provider is down: the last rates are answered instead
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("rates: %s", resp.Status)
		}
		var out map[string]float64
		return out, json.NewDecoder(resp.Body).Decode(&out)
	})
}
```

> [!WARNING]
> An answer can be up to `TTL + StaleIfError` old. Do not use it for data where a stale answer is
> wrong rather than late: permissions, balances, prices at checkout.

#### Cache penetration (negative caching)

**Problem.** Requests for data that does not exist (a deleted user, an invalid id, a scraper trying
ids in a row) are never cached, so every one of them reaches the database.

**Solution.** `Negative` caches "not found" for a short while. While it is cached, `GetOrLoad`
returns the error you chose, so your code keeps checking the same sentinel.

```go
var users = gashe.Policy[User]{
	TTL: time.Hour,
	Negative: gashe.Negative{
		If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) }, // which errors mean "no such data"
		Err: sql.ErrNoRows, // what GetOrLoad returns while the answer is cached
		TTL: time.Minute,
	},
}

_, err := gashe.GetOrLoad(ctx, c, "user:999", users, load)
if errors.Is(err, sql.ErrNoRows) { // true on the first call and on cached ones
	...
}
```

**Scope.** A negative answer is a stored entry: with Redis it protects the database from all
processes, with `memory` from this process only.

**Example.** A 404 for a missing user: a scraper walking ids now hits the cache, not PostgreSQL. With pgx, match `pgx.ErrNoRows` instead.

```go
var profiles = gashe.Policy[User]{
	TTL: time.Hour,
	Negative: gashe.Negative{
		If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
		Err: sql.ErrNoRows,
		TTL: 30 * time.Second,
	},
}

func (s *Service) ProfileHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := gashe.GetOrLoad(r.Context(), s.cache, "user:"+id, profiles, func(ctx context.Context) (User, error) {
		return s.repo.User(ctx, id) // returns sql.ErrNoRows for a missing id
	})
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(u)
}
```

> [!WARNING]
> Data created after a negative answer was cached stays "not found" until `Negative.TTL` ends.
> Keep it short, or `Delete` the key or invalidate its tag when you create the data.

### Load on the source

#### Cache stampede inside a process

**Problem.** A key is missing and a thousand goroutines ask for it at the same moment; each of them
calls the loader, and the database gets a thousand identical queries. Also known as the dogpile
effect.

**Solution.** Automatic. Concurrent `GetOrLoad` calls with the same key and type in one process
share one lookup and one loader run. The run uses the context of the call that started it; a
waiting call returns its own `ctx.Err()` if its context ends first.

**Scope.** This process only, with any backend. Across processes see `LockOnMiss` below.

**Example.** A product page: when one product goes viral, thousands of concurrent requests for it run one query.

```go
func (s *Service) ProductHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := gashe.GetOrLoad(r.Context(), s.cache, "product:"+id, products, func(ctx context.Context) (Product, error) {
		return s.repo.Product(ctx, id) // runs once for all concurrent requests of this pod
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(p)
}
```

#### Cache stampede across processes

**Problem.** The same as above, but you run fifty pods: each pod still loads the missing key once,
and the database gets fifty identical queries.

**Solution.** `LockOnMiss`: on a miss, one process takes a lock in the backend and runs the loader;
the others wait and then read its result. The value is the longest the lock is held, in case the
holder dies. The lock is taken only on a miss, never on a hit.

```go
var reports = gashe.Policy[Report]{TTL: time.Hour, LockOnMiss: 30 * time.Second}
```

**Scope.** All processes that use the same Redis. With `memory` the lock covers this process only
and adds nothing to the automatic sharing above.

**Example.** A dashboard aggregate over millions of rows: on a cold cache, one pod runs the query, the others wait for it.

```go
var stats = gashe.Policy[Stats]{TTL: 10 * time.Minute, LockOnMiss: time.Minute}

func (s *Service) Stats(ctx context.Context, day string) (Stats, error) {
	return gashe.GetOrLoad(ctx, s.cache, "stats:"+day, stats, func(ctx context.Context) (Stats, error) {
		var st Stats
		err := s.db.QueryRowContext(ctx,
			"SELECT count(*), coalesce(sum(amount), 0) FROM orders WHERE created_at::date = $1", day).
			Scan(&st.Orders, &st.Revenue)
		return st, err
	})
}
```

> [!WARNING]
> Make `LockOnMiss` longer than the slowest load, or a second process starts loading when the lock
> expires. Waiting calls wait as long as their `ctx` allows: give requests a deadline.

#### N+1 queries on lists (batch loading)

**Problem.** A page shows 50 products. Loading them one by one with `GetOrLoad` costs 50 round
trips to the cache and, on a cold cache, 50 queries to the database.

**Solution.** `gashe.GetOrLoadMany` reads all keys in one backend call and calls the loader once
with the keys that were missing, so it can fetch them in one query. What the loader returns is
stored in one call; keys it leaves out are absent from the result and are not stored.

```go
products, err := gashe.GetOrLoadMany(ctx, c, ids, catalog,
	func(ctx context.Context, missing []string) (map[string]Product, error) {
		return s.repo.ProductsByID(ctx, missing) // the products found, by id
	})
```

It reads and writes the same entries as `GetOrLoad` with the same policy, so both can serve the
same keys. It supports `TTL`, `TTLFunc`, `Jitter`, `Tags`, `CacheIf` and `Prefix`.

**Scope.** Stored results are seen by every process with Redis, by this process only with `memory`.
The calls themselves are not shared, even inside one process.

**Example.** The cart page: the products of the cart, one SQL query for those not cached.

```go
var catalog = gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.1, Prefix: "product:"}

func (s *Service) CartProducts(ctx context.Context, ids []string) (map[string]Product, error) {
	return gashe.GetOrLoadMany(ctx, s.cache, ids, catalog, func(ctx context.Context, missing []string) (map[string]Product, error) {
		found, err := s.repo.Products(ctx, missing) // SELECT ... WHERE id = ANY($1)
		if err != nil {
			return nil, err
		}
		out := make(map[string]Product, len(found))
		for _, p := range found {
			out[p.ID] = p
		}
		return out, nil
	})
}
```

> [!NOTE]
> `StaleWhileRevalidate`, `StaleIfError`, `LockOnMiss` and `Negative` work per key and make
> `GetOrLoadMany` return `ErrInvalidOptions`; use a policy without them. Concurrent calls are not
> shared either: two requests for the same cold list both call the loader.

#### Cache avalanche (TTL jitter)

**Problem.** Many keys are stored at the same time (after a deploy, a warm-up, a batch job) and
therefore expire in the same second, and the database gets all of their loads at once.

**Solution.** `Jitter` spreads every TTL randomly by up to this share.

```go
var products = gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.1} // 54 to 66 minutes
```

**Scope.** Each stored entry gets its own spread, wherever it is stored.

**Example.** Warming the product catalog at startup: without `Jitter` all of it would expire in the same minute an hour later.

```go
var catalog = gashe.Policy[Product]{TTL: time.Hour, Jitter: 0.2}

func (s *Service) WarmUp(ctx context.Context, ids []string) {
	for _, id := range ids {
		_, err := gashe.Reload(ctx, s.cache, "product:"+id, catalog, func(ctx context.Context) (Product, error) {
			return s.repo.Product(ctx, id)
		})
		if err != nil {
			slog.WarnContext(ctx, "warm-up", "product", id, "error", err)
		}
	}
}
```

#### Hot keys and network round trips (near cache)

**Problem.** One key is read tens of thousands of times a second: every read is a network round
trip, and all of them land on the one Redis node that holds the key.

**Solution.** `nearcache` puts a memory cache in front of the shared backend. Reads come from memory
for `LocalTTL`; writes and deletes go to the shared backend and drop the local copy. Locks,
counters and TTLs always use the shared backend.

```go
import "github.com/eugeneliukindev/gashe/backend/nearcache"

local := memory.New(memory.Options{MaxEntries: 10_000})
defer local.Close()
backend := nearcache.New(local, gasheredis.New(client), nearcache.Options{LocalTTL: 5 * time.Second})
c := gashe.New(backend, gashe.Options{})
```

The price is staleness: another process may keep answering with a value this process already
changed for up to `LocalTTL`, and an invalidated tag reaches other processes within `LocalTTL` too. Keep it
short.

**Scope.** The local level belongs to each process; the shared level is seen by all processes.

**Example.** Feature flags read on every request: with `nearcache`, each pod asks Redis about them about once per `LocalTTL`.

```go
var flags = gashe.Policy[Flags]{TTL: time.Minute}

func (s *Service) Flags(ctx context.Context) (Flags, error) {
	return gashe.GetOrLoad(ctx, s.cache, "flags", flags, func(ctx context.Context) (Flags, error) {
		var f Flags
		err := s.db.QueryRowContext(ctx, "SELECT new_checkout, dark_mode FROM flags").Scan(&f.NewCheckout, &f.DarkMode)
		return f, err
	})
}
```

> [!WARNING]
> The local level lives in every process: with 50 pods, `MaxEntries` of the local `memory.Backend` is
> held 50 times, and the RAM of each pod has to fit it. Bound it with `MaxBytes`, not only entries, and keep
> `LocalTTL` short: it is also how long other pods may answer with a stale value.

### Storage

#### Key collisions in a shared Redis

**Problem.** Several services or environments share one Redis, and their keys collide.

**Solution.** `gashe.WithPrefix` keeps every key of a backend under a prefix; `Keys` and `All`
yield keys without it, and `DeleteMatch` never touches other prefixes.

```go
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), "orders:prod:"), gashe.Options{})
```

**Example.** One Redis for several services and environments: every service gets its own namespace.

```go
prefix := os.Getenv("SERVICE") + ":" + os.Getenv("ENV") + ":" // "orders:staging:"
c := gashe.New(gashe.WithPrefix(gasheredis.New(client), prefix), gashe.Options{})
```

#### Large values

**Problem.** Big values (long lists, rendered pages) take memory in Redis and network on every read.

**Solution.** `gashe.Gzip` compresses what another codec encodes.

```go
c := gashe.New(backend, gashe.Options{Codec: gashe.Gzip{Inner: gashe.JSON{}}})
```

A compressed value is no longer a plain number, so `Incr` does not work on it.

Two more codecs make values smaller without compression:

- `github.com/eugeneliukindev/gashe/codec/msgpack` is a separate module with `msgpack.Codec{}`, based on
  [vmihailenco/msgpack v5](https://github.com/vmihailenco/msgpack). MessagePack is binary: it is
  smaller than JSON and faster to decode. It works inside `Gzip` too: `gashe.Gzip{Inner: msgpack.Codec{}}`.
- `gashe.Raw{}` stores `[]byte` and `string` as they are, without the quotes and base64 of JSON, and
  hands other types to `Inner` (JSON by default). `GetOrLoad` stores its result inside a record of
  its own, so `Raw` helps only `Get`, `Set` and the other basic calls, for example for a page that
  is already rendered.

Values in MessagePack are binary too, so `Incr` does not work on them either.

**Example.** A rendered category page of a few hundred kilobytes, stored compressed.

```go
var pages = gashe.Policy[string]{TTL: 10 * time.Minute}

func (s *Service) CategoryPage(ctx context.Context, slug string) (string, error) {
	return gashe.GetOrLoad(ctx, s.pageCache, "page:category:"+slug, pages, func(ctx context.Context) (string, error) {
		return s.renderCategory(ctx, slug)
	})
}

// s.pageCache = gashe.New(backend, gashe.Options{Codec: gashe.Gzip{}})
```

> [!CAUTION]
> Changing the codec, like changing `Prefix`, makes every stored entry unreadable: all of them
> become misses at once and the database gets the load. Roll such changes out gradually, and keep
> `LockOnMiss` on for hot keys.

### Protecting dependencies

#### Mutual exclusion across processes (distributed lock)

**Problem.** Some work must never run twice at the same time across your processes: a daily
report, a payment, a migration step.

**Solution.** `Lock` and `TryLock` take a lock in the backend; it works like `sync.Mutex`. The
duration frees the lock if its holder dies; `Extend` keeps it for work that runs longer than
planned.

```go
lease, err := c.Lock(ctx, "report:daily", time.Minute) // waits while someone holds it
if err != nil {
	return err
}
defer lease.Unlock(ctx)

for _, part := range parts {
	build(part)
	if err := lease.Extend(ctx, time.Minute); err != nil { // gashe.ErrLockLost if it expired
		return err
	}
}

lease, ok, err := c.TryLock(ctx, "report:daily", time.Minute) // does not wait
if !ok {
	return errBusy
}
```

- `Lock` checks the busy lock every `Options.LockPollInterval` (100ms by default) and stops with
  `ctx.Err()` when `ctx` ends.
- `Unlock` releases the lock only if it is still yours, even after `ctx` was cancelled.
- If the backend fails, `Lock` and `TryLock` return its error: whether to run without a lock is your
  call.

**Scope.** All processes that use the same Redis. With `memory` the lock holds inside this process
only.

**Example.** Rebuilding the search index from a cron that runs on every pod: one pod rebuilds, the others skip.

```go
func (s *Service) RebuildIndex(ctx context.Context) error {
	lease, ok, err := s.cache.TryLock(ctx, "job:reindex", time.Minute)
	if err != nil || !ok {
		return err // another pod is rebuilding, or Redis is down
	}
	defer lease.Unlock(ctx)
	for offset := 0; ; offset += 1000 {
		n, err := s.indexBatch(ctx, offset, 1000)
		if err != nil || n == 0 {
			return err
		}
		if err := lease.Extend(ctx, time.Minute); err != nil {
			return err // the lock was lost: stop, another pod may have started
		}
	}
}
```

> [!CAUTION]
> Locks are advisory, see [Known limits](#known-limits). If a lock guards money or other data where
> two writers cause damage, also make the write itself safe to repeat (idempotent).

#### Overload and abuse (rate limiting)

**Problem.** A client, a user or a bug calls an expensive endpoint far more often than the system
can take.

**Solution.** `RateLimit` counts a call for a key and returns `gashe.ErrRateLimited` once the key is
over its limit. Call it before the work it guards.

```go
if err := c.RateLimit(ctx, "user:"+id, gashe.PerMinute(100)); err != nil {
	return err // gashe.ErrRateLimited
}

// Fixed window with a ban: 5 login attempts a minute, then 15 minutes of silence.
err := c.RateLimit(ctx, "login:"+ip, gashe.FixedWindow{Calls: 5, Per: time.Minute, BanFor: 15 * time.Minute})
```

- `gashe.SlidingWindow{Calls, Per}` and its shortcuts `PerSecond`, `PerMinute`, `PerHour`: about `Calls`
  calls in any span of `Per`. It keeps two counters per key and assumes the calls of the previous
  span were spread evenly. Refused calls are not counted.
- `gashe.FixedWindow{Calls, Per, BanFor}`: at most `Calls` calls per span of `Per` that starts with the
  first call; the first call over the limit blocks the key for `BanFor` (default `Per`).

**Scope.** With Redis, one limit for all processes. With `memory`, each process counts only its own
calls: with 10 pods the real limit is 10 times higher.

**Example.** An HTTP middleware: 600 requests a minute per API key, then 429.

```go
func RateLimited(c *gashe.Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := "api-key:" + r.Header.Get("X-API-Key")
		if err := c.RateLimit(r.Context(), key, gashe.PerMinute(600)); errors.Is(err, gashe.ErrRateLimited) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

> [!NOTE]
> `SlidingWindow` is an estimate and can let slightly more than `Calls` through under bursty
> traffic. `FixedWindow` can let up to twice `Calls` through around the border of two windows.

#### Cascading failures (circuit breaker)

**Problem.** A dependency starts failing or timing out; every request still waits for it, threads
and connections pile up, and the failure spreads to your service and its callers.

**Solution.** `CircuitBreaker` counts calls and failures of the last `Window`. When enough of them
fail it opens: for `OpenFor` every call gets `gashe.ErrCircuitOpen` at once. Then it is half-open:
it lets `HalfOpenCalls` calls through, the first that succeeds closes it, one that fails opens it
again. The
breaker lives in the memory of the `Cache`, so checking it costs no network round trip; every
process judges the dependency by its own calls.

```go
var db = gashe.BreakerOptions{
	FailurePercent: 50,               // open when 50% of calls fail
	MinCalls:       20,               // but only after at least 20 calls
	Window:         time.Minute,      // counted over the last minute
	OpenFor:        30 * time.Second, // stay open this long
	HalfOpenCalls:  3,                // then let 3 test calls through; 0 = just close after OpenFor
	IsFailure:      func(err error) bool { return !errors.Is(err, sql.ErrNoRows) }, // default: any error
}

done, err := c.CircuitBreaker("db", db)
if err != nil {
	return err // gashe.ErrCircuitOpen: do not even try
}
rows, err := queryDB(ctx)
done(err) // report how it went: nil or the error
```

Call `done` once. A call you never report is not counted. Use the same options for the same key.

**Scope.** Each process has its own breaker, with any backend.

**Example.** Calls to a payment provider: while it is failing, checkout answers at once instead of hanging on timeouts.

```go
var payments = gashe.BreakerOptions{
	FailurePercent: 50,
	MinCalls:       10,
	Window:         30 * time.Second,
	OpenFor:        15 * time.Second,
	HalfOpenCalls:  1,
}

func (s *Service) Charge(ctx context.Context, p Payment) error {
	done, err := s.cache.CircuitBreaker("payments-api", payments)
	if err != nil {
		return ErrPaymentsUnavailable // show "try again in a minute" instead of a spinner
	}
	err = s.provider.Charge(ctx, p)
	done(err)
	return err
}
```

> [!WARNING]
> Each process has its own breaker: with 50 pods, each opens after its own failures. The state of
> every key is kept in memory for the life of the `Cache`, so use a few fixed keys (`"db"`,
> `"payments-api"`), not user ids or URLs.

### Operations

#### Cache failure

**Problem.** Redis is down or slow, and the service must not go down with it.

**Solution.** Built in. If the backend fails, `GetOrLoad` calls the loader, returns its result
without storing it, and sends the cache error to `OnError`; `RateLimit` lets the call through.
Bound how long a slow Redis can hold a request with go-redis timeouts (`ReadTimeout`,
`WriteTimeout`, see [Redis](#redis)).

**Example.** Redis is down: requests keep being served from the database, and you see it in the logs.

```go
c := gashe.New(gasheredis.New(client), gashe.Options{
	OnError: func(ctx context.Context, err error) {
		slog.WarnContext(ctx, "cache unavailable, serving from the database", "error", err)
	},
})
```

> [!WARNING]
> When Redis is down, coordination between processes is down too: `LockOnMiss` no longer holds
> back other pods, and every pod sends its misses to the database. Put a `CircuitBreaker` in front
> of the database for that case.

#### Hit ratio and monitoring

**Problem.** You cannot tell whether the cache helps: how often it answers, and how often it falls
back or refreshes.

**Solution.** `OnResult` receives how every `GetOrLoad` call was answered.

**Scope.** `OnResult` sees the calls of this process; sum the metrics of all pods.

```go
c := gashe.New(backend, gashe.Options{
	OnResult: func(ctx context.Context, key string, r gashe.Result) {
		cacheResults.WithLabelValues(r.String()).Inc() // hit, miss, stale-while-revalidate, stale-if-error, negative-hit
	},
	OnError: func(ctx context.Context, err error) {
		slog.WarnContext(ctx, "cache degraded", "error", err)
	},
})
```

> [!WARNING]
> `OnResult` and `OnError` run synchronously on every call. Keep them fast: count, do not write to
> a network or a slow log on the request path.

#### Graceful shutdown

**Problem.** `StaleWhileRevalidate` reloads in the background. When the service stops, such a
reload may still be running: it is cut off halfway or writes to a backend that is already closed.

**Solution.** Call `c.Shutdown(ctx)` before closing the backend. It stops starting background
reloads and waits for the running ones until `ctx` is done. The cache keeps answering; stale
results are just no longer reloaded.

**Scope.** The background reloads of this process.

**Example.** The end of `main`: stop taking requests, let reloads finish, then close Redis.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
_ = srv.Shutdown(ctx) // the HTTP server
if err := c.Shutdown(ctx); err != nil {
	slog.Warn("background reloads did not finish", "error", err)
}
_ = client.Close()
```

### Known limits

- **Cache-aside race.** A read can load an old value from the database, a write can update the
  database and delete the key, and then the read stores its old value; it stays until its TTL ends.
  gashe does not prevent this. Keep TTLs of data that changes often short, or invalidate its tag
  after the write.
- **Locks are advisory.** If a lock holder stops for longer than the lock TTL (a long GC pause, a
  frozen VM), another process can take the lock while the first still works. `Extend` reports
  `gashe.ErrLockLost` when it notices.
- **Clocks.** Freshness is decided by the clock of the process that reads, against the time the
  writing process stored. Keep the clocks in sync (NTP).

## Cache Invalidation

Entries expire by TTL. To change them earlier:

```go
gashe.Reload(ctx, c, "user:42", users, loadUser) // load and store a new result now
c.InvalidateTags(ctx, "user:42")                  // every entry with the tag becomes a miss
c.Delete(ctx, "user:42")
c.DeleteMany(ctx, "user:1", "user:2")
c.DeleteMatch(ctx, "user:*")                      // scans the backend: avoid on large Redis
```

To invalidate everything of one kind after a code change, change the `Prefix` in its options.

> [!CAUTION]
> A new `Prefix` starts with an empty cache: every key misses at once. Expect the load of a cold
> cache after such a deploy.

### Invalidating by tag

**Problem.** One change makes many entries stale: a user changed, and so did their profile, their
orders page and their settings. Deleting them by pattern scans the whole backend, which is slow in
Redis.

**Solution.** `Tags` and `InvalidateTags`. Every tag has a generation counter; an entry remembers
the generations of its tags when it is stored, and `InvalidateTags` increments them, so every entry
with the tag becomes a miss. Invalidating costs one write per tag, and the lookup reads the entry and
its generations in one call. Old entries are never read again and expire by TTL.

```go
profile := gashe.Policy[Profile]{TTL: time.Hour, Tags: []string{"user:42"}}
orders := gashe.Policy[[]Order]{TTL: time.Hour, Tags: []string{"user:42", "orders"}}

p, err := gashe.GetOrLoad(ctx, c, "profile:42", profile, loadProfile)
o, err := gashe.GetOrLoad(ctx, c, "orders:42", orders, loadOrders)

c.InvalidateTags(ctx, "user:42") // both entries are misses now
c.InvalidateTags(ctx, "orders")  // every orders entry is a miss now
```

**Scope.** With Redis, every process sees the invalidation at once, through `nearcache` within
`LocalTTL`. With `memory`, only this process.

**Example.** After an UPDATE in PostgreSQL, everything cached about the user reloads: profile, orders page, settings.

```go
func (s *Service) Rename(ctx context.Context, userID int64, name string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = $2", name, userID); err != nil {
		return err
	}
	return s.cache.InvalidateTags(ctx, fmt.Sprint("user:", userID)) // after the write, not before
}
```

> [!NOTE]
> Tag counters never expire: every tag name is one small key forever. Tags per user (`user:42`) are
> fine; tags per request are not. Each tag of an entry adds a key to its lookup.

## Errors

| Error | When |
|---|---|
| `gashe.ErrNotFound` | `Get`, `TTL`, `Value.Decode` on a miss |
| `gashe.ErrInvalidTTL` | a negative TTL, or a non-positive TTL in `Expire`, `Lock`, `TryLock`, `Extend` |
| `gashe.ErrInvalidOptions` | `Policy`, a limit or `BreakerOptions` that cannot work |
| `gashe.ErrWrongType` | the entry holds data the operation cannot use, e.g. `Incr` on a JSON object |
| `gashe.ErrLockLost` | `Extend` of a lock that expired |
| `gashe.ErrRateLimited` | `RateLimit` refuses a call |
| `gashe.ErrCircuitOpen` | `CircuitBreaker` refuses a call |

Check them with `errors.Is`. Codec errors are wrapped once with the operation and key
(`gashe: decode "user:42": ...`). Backend errors are returned as the backend reports them; the
Redis backend wraps go-redis errors once, as `gashe/redis: get: dial tcp ...`. Errors from your
loader are returned unchanged, and so is `Negative.Err`.

`GetOrLoad` and `RateLimit` do not return backend errors: they carry on without the cache. Those
errors go to `Options.OnError`, wrapped with the operation and key (`gashe: get "user:42": ...`).
`Lock`, `TryLock`, `Extend` and `InvalidateTags` do return backend errors.

## Testing your code

Use the in-memory backend; turn the background purge off so no goroutine outlives the test:

```go
func newCache(t *testing.T) *gashe.Cache {
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	return gashe.New(mem, gashe.Options{})
}
```

TTLs follow `time.Now`, so on Go 1.25+ tests with `testing/synctest` run in fake time without real
sleeps:

```go
synctest.Test(t, func(t *testing.T) {
	c := newCache(t)
	_ = c.Set(ctx, "k", 1, time.Hour)
	time.Sleep(time.Hour) // instant inside the bubble
	err := c.Get(ctx, "k", new(int)) // gashe.ErrNotFound
})
```

Background refreshes run in goroutines; inside a bubble, `synctest.Wait()` lets them finish.
`Jitter` makes TTLs random: leave it out in tests that check exact TTLs.

To simulate a failing backend, embed a real one and override the methods you want to break:

```go
type downBackend struct{ gashe.Backend }

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
```

## Roadmap

1. Core: `Backend`, in-memory backend, basic API, caching. Done.
2. Redis backend as a separate module. Done.
3. Stale-while-revalidate, stale-if-error, stampede lock, distributed locks. Done.
4. Rate limits and the circuit breaker. Done.
5. One object around `Cache`, iterators. Done.
6. Negative caching, jitter, tags, `OnResult`, lock extension, an in-process breaker, prefixes,
   gzip, a two-level near cache. Done.
7. Batch loading of many keys at once (`GetOrLoadMany`), graceful shutdown. Done.
8. Instant invalidation of near caches through Redis pub/sub.

## Development

The repository holds four modules. Run every command in each of them.

| Module | Holds | Dependencies |
|---|---|---|
| `.` | `gashe`, `gashe/backend/memory`, `gashe/backend/nearcache`, `gashe/backendtest` | none |
| `backend/redis/` | `gashe/backend/redis` | go-redis v9 |
| `backend/redis/integration/` | tests of `gashe/backend/redis` against a real Redis | testcontainers-go, Docker |
| `codec/msgpack/` | `gashe/codec/msgpack` | vmihailenco/msgpack v5 |

The integration module is separate so that testcontainers never reaches users of `gashe/backend/redis`.
Its tests start `redis:7.0.0` in Docker by themselves and fail if Docker is not available. Set
`GASHE_REDIS_IMAGE` to run them against another server, for example
`GASHE_REDIS_IMAGE=valkey/valkey:8 go test ./...`.

The modules support Go 1.23 and newer, five releases at the moment. Tests that need
`testing/synctest` are behind `//go:build go1.25`, so on Go 1.23 and 1.24 they are skipped and the
rest still run. The integration module needs Go 1.25, as testcontainers does.

```bash
go test -race ./...  # Go 1.25+ runs every test, fake-time ones included
golangci-lint run    # golangci-lint v2, config in .golangci.yml
GOENV_VERSION=1.23.12 GOTOOLCHAIN=local go test -race ./...  # the oldest supported Go
```

`backend/redis` and `codec/msgpack` require a released version of the root module. To change the
root and another module together, create a workspace once; `go.work` is not committed:

```bash
go work init . ./backend/redis ./backend/redis/integration ./codec/msgpack
```

The integration module is never published, so its `go.mod` replaces the gashe modules with the
local copies and always tests the code of the repository.

To release, tag the root first (`v0.2.0`), then raise the version the other modules require, and
tag them with their directory in front: `backend/redis/v0.2.0`, `codec/msgpack/v0.2.0`.

The code is under the [MIT license](LICENSE).

Domain terms are defined in `GLOSSARY.md`. `REFERENCE.md` and `REFERENCE.ru.md` list the public API;
a test fails when an exported name is missing from them.
