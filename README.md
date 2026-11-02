# gashe

English | [Russian](README.ru.md)

Caching for Go services: store results of expensive calls, keep answering when a dependency is
slow or down, and guard dependencies with locks, rate limits and circuit breakers, all on one
`*gashe.Cache`.

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/redis # Redis backend, depends on go-redis v9
```

Requires Go 1.23 or newer.

## Why

A cache is more than `Get` and `Set`. Real services need to keep answering while the database is
down, to refresh a popular key without making callers wait, to stop a cold key from sending a
hundred identical queries at once, and to keep working when the cache itself fails. gashe gives
these techniques plain names and keeps them around one object, so the call site stays a function
call.

## Features

- One object: locks, rate limits, circuit breakers and the basic API are methods of
  `*gashe.Cache`; typed caching is `gashe.GetOrLoad(ctx, c, key, opts, load)`, which returns your
  type.
- Old results on demand: refresh in the background, or answer with the last good result when the
  loader fails, all in one stored entry.
- A lock on a miss, so one process loads a cold key while the others wait for it.
- Concurrent calls with the same key share one lookup and one load.
- Distributed locks, sliding and fixed rate limits, a circuit breaker with trial calls.
- Storage backends behind one interface: an in-memory LRU with TTL, and Redis (single node,
  Sentinel or Cluster) in a separate module.
- Values of any type through a pluggable `Codec` (JSON by default).
- When the cache itself fails, your code keeps working and the error goes to your `OnError` hook.
- No global state and no third-party dependencies in the core module; Go 1.23 to the newest release.

## Usage Example

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/memory"
)

type User struct {
	ID   int
	Name string
}

// users says how users are cached; define it once.
var users = gashe.CacheOptions[User]{
	TTL:            time.Hour,      // fresh for an hour
	FallbackWindow: 24 * time.Hour, // if the database is down, answer with the old user for a day
}

func loadUser(ctx context.Context, id int) (User, error) {
	return User{ID: id, Name: "Ann"}, nil // a database call in real code
}

func main() {
	ctx := context.Background()

	mem := memory.New(memory.Options{Size: 10_000})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// Caching: from the cache if it is there, otherwise loadUser.
	u, err := gashe.GetOrLoad(ctx, c, "user:42", users, func(ctx context.Context) (User, error) {
		return loadUser(ctx, 42)
	})
	fmt.Println(u, err)

	// A rate limit before an expensive call.
	if err := c.RateLimit(ctx, "user:42", gashe.PerMinute(100)); err != nil {
		fmt.Println(err) // gashe.ErrRateLimited
	}

	// The basic API.
	_ = c.Set(ctx, "key", 90, 2*time.Hour)
	var n int
	if err := c.Get(ctx, "key", &n); errors.Is(err, gashe.ErrNotFound) {
		fmt.Println("miss")
	}
}
```

## Table of Contents

- [Configuration](#configuration)
- [Available Backends](#available-backends)
- [Basic API](#basic-api)
- [Strategies](#strategies)
  - [GetOrLoad: caching](#getorload-caching)
  - [Locks](#locks)
  - [Rate limit](#rate-limit)
  - [Circuit breaker](#circuit-breaker)
  - [Combining them](#combining-them)
- [Keys](#keys)
- [TTL](#ttl)
- [What can be cached](#what-can-be-cached)
- [Cache Invalidation](#cache-invalidation)
- [Errors](#errors)
- [Testing your code](#testing-your-code)
- [Roadmap](#roadmap)
- [Development](#development)

## Configuration

Everything is configured with option structs. A field you leave out gets its default, so an
empty struct always works.

```go
mem := memory.New(memory.Options{
	Size:          10_000,      // keep at most this many entries; 0 means 1000
	CheckInterval: time.Minute, // how often to throw out expired entries; 0 means every second
})
defer mem.Close()

c := gashe.New(mem, gashe.Options{
	Codec:   gashe.JSON{}, // how values become bytes; leave it out for JSON
	OnError: onError,      // see below
})
```

| `gashe.Options` field | What it is for |
|---|---|
| `Codec` | Turns your values into bytes for the backend and back. JSON unless you set another. |
| `OnError` | `GetOrLoad`, `RateLimit` and `CircuitBreaker` never fail just because the cache is broken: they carry on without it. The cache error is not lost, it goes to this function. Use it to log or count such errors. Without it they are dropped. |

```go
onError := func(ctx context.Context, err error) {
	slog.WarnContext(ctx, "cache degraded", "error", err)
}
```

Create one `Cache` at startup and share it; it is safe for concurrent use. It does not close the
backend: close the backend where you created it.

## Available Backends

### In-memory

`github.com/eugeneliukindev/gashe/memory` keeps entries in process memory: a fixed-size LRU where
every entry may have its own TTL. Expired entries are never returned; they are removed when they are
read and by a background purge every `CheckInterval`. Stored bytes are copied in and out, so
changing a value after `Set` or `Get` never changes the cache.

Patterns in `Keys`, `All` and `DeleteMatch` use Redis glob syntax: `*`, `?`, `[abc]`, `[^a]`,
`[a-z]`, and `\` to escape.

### Redis

`github.com/eugeneliukindev/gashe/redis` is a separate module on top of
[go-redis v9](https://github.com/redis/go-redis), so the core module stays dependency-free. You build
the go-redis client yourself (address, TLS, pool, Sentinel or Cluster are go-redis options) and
close it yourself: the backend does not own it.

```go
import (
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/redis"
)

client := goredis.NewUniversalClient(&goredis.UniversalOptions{Addrs: []string{"localhost:6379"}})
defer client.Close()
c := gashe.New(gasheredis.New(client), gashe.Options{})
```

Notes:

- On a Cluster, `Keys`, `All`, `DeleteMatch`, `KeysCount` and `Clear` visit every master, and
  multi-key operations are sent one command per key, so keys may live in different slots.
- `Keys` and `All` may yield a key more than once, as Redis `SCAN` does.
- `Clear` runs `FLUSHDB`: it removes every key in the selected database, not only gashe entries.
- `Size` reports `MEMORY USAGE`, which includes Redis overhead.
- Redis errors are wrapped once, as `gashe/redis: <op>: <error>`. A missing key becomes
  `gashe.ErrNotFound`; a `WRONGTYPE` reply or a non-integer `Incr` becomes `gashe.ErrWrongType`.

### Your own backend

Implement `gashe.Backend`. It works with bytes only: `Cache` encodes values before they reach the
backend. Read the method comments in `backend.go` for the exact contract: a `ttl` of 0 means no
expiry, a miss is `gashe.ErrNotFound`, and every method must be safe for concurrent use.

Check your backend against the same contract suite the bundled backends pass:

```go
func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // a fresh, empty backend for every subtest
	})
}
```

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
c.Size(ctx, "key")                          // (int64, error); bytes, 0 for a missing key
c.KeysCount(ctx)                            // (int64, error)
c.Ping(ctx)                                 // error
c.Clear(ctx)                                // error

c.Backend()                                 // gashe.Backend, for raw bytes
```

An integer written with `Incr` reads back with `Get` into an `int`, and a JSON number written with
`Set` can be incremented with `Incr`.

## Strategies

| You want | Use |
|---|---|
| to cache results | `gashe.GetOrLoad` with `TTL` |
| to keep answering when the database or an API is down | `GetOrLoad` with `FallbackWindow` |
| users never to wait for a slow reload of a popular key | `GetOrLoad` with `RefreshWindow` |
| only one process to load a missing key, not all of them at once | `GetOrLoad` with `LockOnMiss` |
| a result to be loaded again after N reads | `GetOrLoad` with `MaxReads` |
| a job never to run twice at the same time, across processes | `c.Lock` / `c.TryLock` |
| to limit how often a key may call | `c.RateLimit` |
| to stop calling a dependency that keeps failing | `c.CircuitBreaker` |

### GetOrLoad: caching

`gashe.GetOrLoad` returns the result stored under a key. If there is none, or it is too old, it calls
your function (the **loader**), stores the result and returns it.

```go
var users = gashe.CacheOptions[User]{TTL: time.Hour}

u, err := gashe.GetOrLoad(ctx, c, "user:42", users, func(ctx context.Context) (User, error) {
	return db.LoadUser(ctx, 42) // runs only when the cache cannot answer
})
```

- `"user:42"` is the key. You build it yourself from the arguments.
- `users` is a `CacheOptions[User]`: the rules for this kind of data. Define it once and reuse it.
- The loader returns `User`, so `GetOrLoad` returns `User` too. It is a function rather than a
  method of `c` because Go methods cannot have type parameters before Go 1.27.
- An error from the loader is returned to you as is and is never stored.
- Ten concurrent calls with the same key in one process run the loader once and share the result.
- If the cache itself fails (Redis is down), the loader runs, you get its result, and the cache
  error goes to `OnError`.
- Options that cannot work (a negative duration, say) return `gashe.ErrInvalidOptions`.

A stored result goes through up to three stages:

```
  stored           TTL ends                 TTL + window ends
    |------ fresh ------|--------- old ---------|   gone
       answered from        answered only in
       the cache            the cases you allow
```

- **Fresh**: answered from the cache, the loader is not called.
- **Old**: without extra options it is ignored and the loader runs. `RefreshWindow` and
  `FallbackWindow` say when an old result is still good enough.
- **Gone**: deleted from the cache; the next call runs the loader.

#### The options

**`TTL`** - how long a result stays fresh.

```go
gashe.CacheOptions[User]{TTL: time.Hour} // a plain cache: one hour, then load again
```

**`FallbackWindow`** - for how long after `TTL` an old result answers when your loader fails.
Use it to keep working while the database or an API is down. The loader still runs first; only if
it returns an error you get the old result (and a nil error) instead. The loader error goes to
`OnError`.

```go
// Fresh for 5 minutes. After that we reload; if the API fails, we answer with the old
// result for up to an hour.
gashe.CacheOptions[Rates]{TTL: 5 * time.Minute, FallbackWindow: time.Hour}
```

**`FallbackIf`** - which loader errors `FallbackWindow` covers. Without it, any error. Use it so
that, for example, "not found" is returned as an error and only "API is down" is covered.

```go
FallbackIf: func(err error) bool { return errors.Is(err, ErrAPIDown) },
```

**`RefreshWindow`** - for how long after `TTL` an old result is answered at once while the cache
reloads it in the background. Use it for popular keys: nobody waits for the slow loader, and only
one caller in all your processes runs the reload. A failed reload keeps the old result and goes to
`OnError`.

```go
// Fresh for 10 minutes. In the next 2 minutes callers get the old result immediately and one
// of them reloads it in the background.
gashe.CacheOptions[Feed]{TTL: 10 * time.Minute, RefreshWindow: 2 * time.Minute}
```

**`LockOnMiss`** - when the key is not in the cache at all, only one process runs the loader; the
others wait and then take its result. Use it when a cold key would otherwise send hundreds of
identical queries to the database at once. The value is the longest the lock is held, in case the
process holding it dies. The lock is taken only on a miss, never on a hit.

```go
gashe.CacheOptions[Report]{TTL: time.Hour, LockOnMiss: 30 * time.Second}
```

**`CacheIf`** - decides whether a result is worth storing. Without it, every successful result is
stored. `gashe.NotNil` skips nil pointers, slices and maps.

```go
CacheIf: gashe.NotNil[*User], // do not cache "no such user"
```

**`MinLoadTime`** - store a result only if the loader took longer than this. Fast answers are not
worth the cache space.

```go
MinLoadTime: 100 * time.Millisecond,
```

**`MaxReads`** and **`RefreshAfterReads`** - load a result again after it was read `MaxReads`
times, even while it is fresh. On read number `RefreshAfterReads` it is reloaded in the background
while still being answered. Every read then also updates a counter, so a read costs a write.
`MaxReads` needs a `TTL`.

```go
gashe.CacheOptions[Status]{TTL: 2 * time.Hour, MaxReads: 100, RefreshAfterReads: 90}
```

**`TTLFunc`** - pick `TTL` from the result instead of one fixed value. Return a negative duration
to not store this result.

```go
TTLFunc: func(u User) time.Duration {
	if u.Admin {
		return time.Minute // admins change often
	}
	return time.Hour
},
```

**`Prefix`** - added in front of every key (`v2:user:42`). Change it when the shape of your type
changes, so old entries are not read into the new type.

Options can be combined:

```go
// Fresh for 4 minutes, then 1 minute of background refresh, and if the API is down, the old
// result for up to an hour. All of it is one entry in the cache.
gashe.CacheOptions[Status]{
	TTL:            4 * time.Minute,
	RefreshWindow:  time.Minute,
	FallbackWindow: time.Hour,
}
```

`TTL` may be 0 when a window is set:

- `FallbackWindow: 2 * time.Hour` alone: the loader runs on every call, and the last good result
  answers its errors for up to two hours (a "failover" cache).
- `RefreshWindow: 24 * time.Hour` alone: every call gets the stored result at once and triggers a
  background reload; a key nobody reads for a day expires.

#### Reload

To replace a stored result right now, for example after you updated the user:

```go
u, err := gashe.Reload(ctx, c, "user:42", users, loadUser) // runs the loader and stores its result
```

Service keys: the background reload holds `<key>:refresh` for at most a minute, `LockOnMiss` holds
`<key>:load`, and `MaxReads` counts in `<key>:reads`.

### Locks

A lock makes sure some work never runs twice at the same time for the same key, across every
process that shares the backend. It works like `sync.Mutex`: take it, `defer` the unlock.

```go
unlock, err := c.Lock(ctx, "report:daily", time.Minute) // waits while someone holds it
if err != nil {
	return err
}
defer unlock(ctx)

unlock, ok, err := c.TryLock(ctx, "report:daily", time.Minute) // does not wait
if !ok {
	return errBusy // someone else is building the report
}

locked, err := c.IsLocked(ctx, "report:daily")
```

- The duration frees the lock in case its holder dies; keep it longer than the work.
- `Lock` retries every 100ms and stops with `ctx.Err()` when `ctx` ends.
- `unlock` releases the lock only if it is still yours, and works even after `ctx` was cancelled.

### Rate limit

`RateLimit` counts a call for a key and returns `gashe.ErrRateLimited` once the key is over its
limit. Call it before the work it guards; the limit is shared by every process.

```go
if err := c.RateLimit(ctx, "user:"+id, gashe.PerMinute(100)); err != nil {
	return err // gashe.ErrRateLimited
}
```

Two kinds of limits:

- **`gashe.Sliding{Calls, Per}`**, or the shortcuts `gashe.PerSecond(n)`, `gashe.PerMinute(n)`,
  `gashe.PerHour(n)`: about `Calls` calls in any span of `Per`. It is an estimate: it keeps two
  counters per key and assumes the calls of the previous span were spread evenly. Refused calls are
  not counted.
- **`gashe.Fixed{Calls, Per, BanFor}`**: at most `Calls` calls per span of `Per` that starts with the
  first call. The first call over the limit blocks the key for `BanFor` (default `Per`), counted
  from that call.

```go
c.RateLimit(ctx, "login:"+ip, gashe.Fixed{Calls: 5, Per: time.Minute, BanFor: 15 * time.Minute})
```

If the backend fails, the call is let through and the error goes to `OnError`.

### Circuit breaker

Stops calling something that keeps failing, so it gets time to recover and your callers get an
answer at once instead of waiting for timeouts.

It works in two steps: ask before the call, report after it.

```go
var db = gashe.BreakerOptions{
	FailurePercent: 50,               // open when 50% of calls fail
	MinCalls:       20,               // but only after at least 20 calls
	Window:         time.Minute,      // counted over the last minute
	OpenFor:        30 * time.Second, // stay open this long
	TrialCalls:     3,                // then let 3 test calls through; 0 = just close after OpenFor
	IsFailure:      func(err error) bool { return !errors.Is(err, sql.ErrNoRows) }, // default: any error
}

done, err := c.CircuitBreaker(ctx, "db", db)
if err != nil {
	return err // gashe.ErrCircuitOpen: do not even try
}
rows, err := queryDB(ctx)
done(err) // report how it went: nil or the error
```

- While the breaker is open, every call gets `gashe.ErrCircuitOpen` without running your code.
- After `OpenFor` it lets `TrialCalls` calls through: the first that succeeds closes the breaker,
  one that fails opens it again. If no trial comes within a `Window`, the breaker closes by itself.
- Call `done` exactly once. A call you never report is simply not counted.
- If the backend fails, the breaker counts as closed and the error goes to `OnError`.

### Combining them

```go
// Answer from the cache; on a miss, ask the rate limit and the breaker before calling the API.
// If the API fails or a guard refuses, answer with the last good status for up to an hour.
var statuses = gashe.CacheOptions[Status]{TTL: time.Minute, FallbackWindow: time.Hour}

st, err := gashe.GetOrLoad(ctx, c, "status:"+id, statuses, func(ctx context.Context) (Status, error) {
	if err := c.RateLimit(ctx, "api", gashe.PerSecond(50)); err != nil {
		return Status{}, err
	}
	done, err := c.CircuitBreaker(ctx, "api", apiBreaker)
	if err != nil {
		return Status{}, err
	}
	st, err := api.Status(ctx, id)
	done(err)
	return st, err
})
```

## Keys

Keys are strings you build in code from the call arguments:

```go
gashe.GetOrLoad(ctx, c, fmt.Sprintf("user:%d:lang:%s", id, lang), users, load)
gashe.GetOrLoad(ctx, c, fmt.Sprintf("token:%x", sha256.Sum256([]byte(token))), tokens, load)
```

`GetOrLoad` adds `Prefix` as `prefix:key`. Rate limits and circuit breakers store under their own
prefixes (`srl:`, `rate_limit:`, `circuit_breaker:`), so they never collide with your entries.
Different value types need different keys: reading a key as another type is a decode failure,
which `GetOrLoad` treats as a miss and reloads. When the shape of a cached type changes, change its
`Prefix` (`"v2"`) so old entries are not read into the new type.

## TTL

TTLs are plain `time.Duration` values.

- In `GetOrLoad`, `TTL` is how long a result is fresh, and the entry is kept for `TTL` plus the
  longer window. `TTLFunc` picks the TTL per result. See [the options](#the-options).
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

## Cache Invalidation

Entries expire by TTL. To change them earlier:

```go
gashe.Reload(ctx, c, "user:42", users, loadUser) // load and store a new result now
c.Delete(ctx, "user:42")
c.DeleteMany(ctx, "user:1", "user:2")
c.DeleteMatch(ctx, "user:*")
```

To invalidate everything of one kind after a code change, change the `Prefix` in its options.

## Errors

| Error | When |
|---|---|
| `gashe.ErrNotFound` | `Get`, `TTL`, `Value.Decode` on a miss |
| `gashe.ErrInvalidTTL` | a negative TTL, or a non-positive TTL in `Expire`, `Lock`, `TryLock` |
| `gashe.ErrInvalidOptions` | `CacheOptions`, a limit or `BreakerOptions` that cannot work |
| `gashe.ErrWrongType` | the entry holds data the operation cannot use, e.g. `Incr` on a JSON object |
| `gashe.ErrRateLimited` | `RateLimit` refuses a call |
| `gashe.ErrCircuitOpen` | `CircuitBreaker` refuses a call |

Check them with `errors.Is`. Codec errors are wrapped once with the operation and key
(`gashe: decode "user:42": ...`). Backend errors are returned as the backend reports them; the
Redis backend wraps go-redis errors once, as `gashe/redis: get: dial tcp ...`. Errors from your
loader are returned unchanged.

`GetOrLoad`, `RateLimit` and `CircuitBreaker` do not return backend errors: they carry on without
the cache. Those errors go to `Options.OnError`, wrapped with the operation and key
(`gashe: get "user:42": ...`), so you can log them or count them. Without `OnError` they are
dropped. `Lock` and `TryLock` do return backend errors: whether to run the work without a lock is
your call.

## Testing your code

Use the in-memory backend; turn the background purge off so no goroutine outlives the test:

```go
func newCache(t *testing.T) *gashe.Cache {
	mem := memory.New(memory.Options{CheckInterval: -1})
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

To simulate a failing backend, embed a real one and override the methods you want to break:

```go
type downBackend struct{ gashe.Backend }

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
```

## Roadmap

1. Core: `Backend`, in-memory backend, basic API, caching. Done.
2. Redis backend as a separate module. Done.
3. Old results (background refresh, fallback on error), lock on a miss, read limits, locks. Done.
4. Rate limits and the circuit breaker. Done.
5. One object: guards and the basic API are methods of `Cache`, caching is `gashe.GetOrLoad`;
   iterators for keys. Done.
6. Tags.
7. Observation hooks, a two-level backend (memory in front of Redis), TTL jitter, caching of
   "not found" results.
8. Batch loading of many keys at once.
9. Compression codec, net/http middleware, Prometheus metrics.

## Development

The repository holds three modules. Run every command in each of them.

| Module | Holds | Dependencies |
|---|---|---|
| `.` | `gashe`, `gashe/memory`, `gashe/backendtest` | none |
| `redis/` | `gashe/redis` | go-redis v9 |
| `redis/integration/` | tests of `gashe/redis` against a real Redis | testcontainers-go, Docker |

The integration module is separate so that testcontainers never reaches users of `gashe/redis`.
Its tests start `redis:7.0.0` in Docker by themselves and fail if Docker is not available.

The modules support Go 1.23 and newer, five releases at the moment. Tests that need
`testing/synctest` are behind `//go:build go1.25`, so on Go 1.23 and 1.24 they are skipped and the
rest still run. The integration module needs Go 1.25, as testcontainers does.

```bash
go test -race ./...  # Go 1.25+ runs every test, fake-time ones included
golangci-lint run    # golangci-lint v2, config in .golangci.yml
GOENV_VERSION=1.23.12 GOTOOLCHAIN=local go test -race ./...  # the oldest supported Go
```

`redis/go.mod` and `redis/integration/go.mod` replace the gashe modules with local copies; remove
the `replace` lines and require tagged versions before publishing the Redis module.

Design decisions are recorded in `docs/adr`, domain terms in `GLOSSARY.md`.
