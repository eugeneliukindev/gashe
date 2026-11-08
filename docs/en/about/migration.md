# Migration { #migration }

What to change when you upgrade, one section per release that breaks code or stored data, newest
first. The full list of changes is the
[changelog](../changelog.md); why `0.x` breaks is in the [version policy](version-policy.md).

| From | Read |
|---|---|
| `0.6.x` | [Unreleased](#unreleased) |
| `0.5.x` | [0.6.0](#060), then the section above it |
| `0.4.x` | [0.5.0](#050), then the sections above it |
| `0.3.x` | [0.4.0](#040), then the sections above it |
| `0.2.x` or `0.1.x` | [0.3.0](#030), and from `0.1.x` also [0.2.0](#020), then the sections above them |

`v0.1.0` and `v0.2.0` are retracted, so `go get` does not pick them: from `0.1.x` upgrade straight to
`v0.3.0` or later and apply both the 0.2.0 and the 0.3.0 changes.

## Unreleased { #unreleased }

| Before | After |
|---|---|
| `CacheIf: f` | `Store: gashe.Store[V]{If: f}` |

## 0.6.0 { #060 }

| Before | After |
|---|---|
| Go 1.23 or 1.24 | Go 1.25 or newer |

## 0.5.0 { #050 }

**Code:**

| Before | After |
|---|---|
| `httpcache.Options{Key: keyOf}` | `httpcache.Options{Vary: httpcache.Vary{Custom: keyOf}}` |
| a `Key` function that put the user into the key | `Vary.User`, which also lets requests with `Authorization` and `Cookie` headers in |
| `backend/redis` on go-redis `v9.0.0` | go-redis `v9.18.0` or newer |
| keys of yours that start with `_` | allowed now; keys of yours must not start with `__` |
| `StaleWhileRevalidate: d` | `Stale: gashe.Stale{WhileRevalidate: d}` |
| `StaleIfError: d`, `StaleIf: f` | `Stale: gashe.Stale{IfError: gashe.StaleIfError{For: d, IsFailure: f}}` |
| `StaleIfSlow: s` (within `StaleIfError: d`) | `Stale: gashe.Stale{IfSlow: gashe.StaleIfSlow{For: d, After: s}}`: `For` is a window of its own now |
| `TTL: d, Jitter: j` | `TTL: gashe.TTL{For: d, Jitter: j}` |
| `LockOnMiss: d` | `Stampede: gashe.Stampede{LockFor: d}` |
| `MaxConcurrentLoads: n` | `Stampede: gashe.Stampede{MaxLoads: n}` |
| `Fallback: &gashe.FallbackOptions{...}` | `Fallback: gashe.Fallback{...}` |
| `gashe.Options{OnResult: r, OnError: e}` | `gashe.Options{Hooks: gashe.Hooks{OnResult: r, OnError: e}}` |
| `gashe.CircuitBreakerOptions{...}`, `Options{CircuitBreaker: &b}` | `gashe.CircuitBreaker{...}`, `Options{CircuitBreaker: b}` |
| a TTL function that returns a negative TTL to leave a result out | `CacheIf` |
| `Negative{If: f, Err: e, TTL: d}` | `Negative{IsNotFound: f, TTL: d}`; check a "not found" with `errors.Is(err, gashe.ErrNotFound)`: the loader error is wrapped in it when the loader ran, so `errors.Is(err, e)` holds then too |

**Behaviour:**

| Case | Before | After |
|---|---|---|
| a request with a `Cookie` header | cached by `httpcache` | goes past the cache, unless `Vary.Cookies` lists the cookies the page depends on or `Vary.User` stores pages per user |
| a response with a `Vary` header | never stored | stored when its `Vary` names only headers of `Options.Vary` |
| a `Bus` fails to read a message | - | its near cache drops every copy, as after a reconnect |
| concurrent calls for one key in a process | always share one load | share one only with `Stampede: gashe.Stampede{SingleFlight: true}`; set it to keep the old behavior |

**Stored data:** the keys the cache keeps for itself start with `__` instead of `_`: `__tag:`,
`__load:`, `__refresh:`, `__rate:sliding:`, `__rate:fixed:`.

| What | After the upgrade |
|---|---|
| entries with tags | load once: their tag counters start again under the new keys |
| rate limit counters | start from zero |
| locks of `LockOnMiss` and `StaleWhileRevalidate` | a process on the old version and one on the new do not see each other's locks while both run |
| the old `_` keys | stay until they expire; tag counters never expire: delete them with `DeleteMatch("_tag:*")` once every process runs the new version |

## 0.4.0 { #040 }

**Code:**

| Before | After |
|---|---|
| `Codec: gashe.Gzip{Inner: c}` | `Codec: c, Compressor: gzip.New()`, with the package `gashe/compress/gzip` |
| `gashe.Raw{}` | `raw.Codec{}`, with the package `gashe/codec/raw` |

**Stored data:** nothing reloads. `Compressor: gzip.New()` reads the entries `gashe.Gzip` stored: both are plain gzip of what the codec writes.

## 0.3.0 { #030 }

**Code:**

| Before | After |
|---|---|
| `WithPrefix(...)` | `gashe.New(b, gashe.Options{Prefix: "orders:"})`; `Policy.Prefix` adds up after it |
| `WithBreaker(...)` | `gashe.New(b, gashe.Options{CircuitBreaker: &opts})` |
| `BreakerOptions` | `CircuitBreakerOptions` |
| `Options.Namespace` | removed: the keys of the cache live under `Options.Prefix`; keys of yours must not start with `_` |
| `Policy{TTLFunc: f}` | `Policy{TTL: func(v V) time.Duration {...}}`; `TTL` also takes a `time.Duration` or a string such as `"1h"`, another type returns `ErrInvalidOptions` |
| `Incr` on `Cache` and `Backend` | `IncrBy`; a backend of yours renames its method too |
| `backend/redis` or `codec/msgpack` with an older gashe | they require gashe `v0.3.0` |

**Stored data:** the keys the cache keeps for itself start with `_` under `Options.Prefix`: `_tag:`,
`_load:`, `_refresh:`, `_rate:sliding:`, `_rate:fixed:`.

| What | After the upgrade |
|---|---|
| tagged entries | reload once |
| rate limit counters | start from zero |

## 0.2.0 { #020 }

**Code:**

| Before | After |
|---|---|
| `OnResult: func(ctx context.Context, key string, r gashe.Result)` | `OnResult: func(ctx context.Context, o gashe.Outcome)`: the key, the `Result` and how long the loader ran |
| `nearcache.Backend.Close` | also stops listening to the `Bus` and waits for it; it still closes neither backend |

**Stored data:** the keys the cache keeps for itself live under `Options.Namespace`, `gashe:` by
default.

| Before | After | After the upgrade |
|---|---|---|
| `<key>:load`, `<key>:refresh` | `gashe:load:<key>`, `gashe:refresh:<key>` | - |
| `srl:`, `rate_limit:` | `gashe:rate:sliding:`, `gashe:rate:fixed:` | rate limit counters start from zero |
| tag counters | keep their keys, unless you set a namespace | - |
| entries from 0.1.0 | read as they are | - |
