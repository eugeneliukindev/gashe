# Changelog

Notable changes of every release, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/): while the version is `0.x`, a minor release may change
the API. The root module, `backend/redis`, `codec/msgpack` and `compress/zstd` are released with
their own tags (`v0.4.0`, `backend/redis/v0.3.0`, `compress/zstd/v0.1.0`); a section names the
module it is about.

## [Unreleased]

### Added

- `Policy.Stampede.MaxLoads`: at most this many loads of one kind of data, its type and `Prefix`,
  run at once in a process; a call over the limit waits as long as its `ctx` allows and then gets
  `ErrOverloaded`, and one an old result can answer gets it at once. `httpcache` answers a request
  held back with `503`.

- `Policy.Fallback`, a `gashe.Fallback`: while the backend fails or `CircuitBreaker` is open, every load of the policy,
  through `GetOrLoad`, `GetOrLoadMany`, `Reload` or `httpcache`, keeps what the loader returns in a
  backend of its own, usually memory, and answers from it, so the database gets one load per key and
  process instead of one per call. `MaxTTL`, 30 seconds by default, caps how long an entry stays
  there, per kind of data. The basic API and locks do not use it. `ResultFallback` reports such an
  answer.
- `httpcache.Vary` says which parts of a request a page depends on, as the `Vary` header does for
  headers, and so what its key is made of: `Headers`, such as `Accept-Language` or
  `X-Forwarded-Host`; `Query`, the parameters that matter, so `utm_source` and random ones make no
  new entries; `Cookies`, the cookies a page depends on, so an analytics cookie no longer keeps a
  request out; `User`, for pages per user; and `Custom`. A response whose `Vary` header names only
  headers of `Options.Vary` is now stored; before, any `Vary` kept it out.
- `backend/redis`: `NewTrackingBus`, a `nearcache.Bus` on Redis client side caching
  (`CLIENT TRACKING`): Redis itself reports every change of a key under a prefix, so near caches see
  changes made past gashe, by `redis-cli` or another service, and `FLUSHDB`. It needs Redis 6 or
  Valkey and does not work on a Cluster, where `NewBus` stays the choice.
- The documentation lists every header `httpcache` reads and sets, and how it works behind a reverse proxy.

### Changed

- Fields that belong together are grouped, and sorted by how often they are set:
  `Policy{TTL: d, Jitter: j}` becomes `Policy{TTL: gashe.TTL{For: d, Jitter: j}}`;
  `LockOnMiss: d` becomes `Stampede: gashe.Stampede{LockFor: d}`;
  `Options{OnResult: r, OnError: e}` becomes `Options{Hooks: gashe.Hooks{OnResult: r, OnError: e}}`.
- Concurrent calls for one key in a process share one load only with
  `Stampede: gashe.Stampede{SingleFlight: true}`; before, they always did. Set it to keep the old
  behavior. Leave it off when the loader reads from `ctx` what the key does not name: a shared load
  runs with the `ctx` values of the call that started it.
- `CircuitBreakerOptions` is `CircuitBreaker`, and `Options.CircuitBreaker` is a value, not a
  pointer: its zero value, with no `FailurePercent`, means no breaker.
- `Negative.If` is `Negative.IsNotFound`, named as `IsFailure` is.
- A negative TTL from the function of `TTL.For` no longer means "do not store" silently: the
  result is still not stored, and `OnError` gets `ErrInvalidOptions`. Leave a result out with
  `CacheIf`.
- The stale options of `Policy` are one struct, `Policy.Stale`, and each way to answer with the
  stale result has its own window after `TTL`:
  `StaleWhileRevalidate: d` becomes `Stale: gashe.Stale{WhileRevalidate: d}`;
  `StaleIfError: d, StaleIf: f` becomes `Stale: gashe.Stale{IfError: gashe.StaleIfError{For: d, IsFailure: f}}`;
  `StaleIfSlow: s` becomes `Stale: gashe.Stale{IfSlow: gashe.StaleIfSlow{For: d, After: s}}`, where
  `For` is a window of its own, so `IfSlow` no longer needs `IfError`.
- `Negative.Err` is removed: a "not found" of `Policy.Negative` returns `ErrNotFound`, wrapping
  the loader error when it loaded and alone while it is cached, so callers check
  `errors.Is(err, gashe.ErrNotFound)`. `Negative{If: f, Err: e, TTL: d}` becomes
  `Negative{IsNotFound: f, TTL: d}`.
- The documentation moved from the README to a site, https://eugeneliukindev.github.io/gashe/,
  built from `docs/` with Zensical, with code from `examples/` and outputs of real runs, curl
  against `httpcache` included. The README keeps a short example.
- `httpcache.Options.Key`, a function, becomes `httpcache.Options.Vary`, a `httpcache.Vary`:
  `Key: keyOf` becomes `Vary: httpcache.Vary{Custom: keyOf}`, and a key that held the user becomes
  `Vary.User`, which also lets requests with `Authorization` and `Cookie` headers in.
- The keys the cache keeps for itself start with `__` instead of `_`: `__tag:`, `__load:`,
  `__refresh:`, `__rate:sliding:`, `__rate:fixed:`; keys of yours may start with `_` now, not with
  `__`. After the upgrade, tagged entries load once and rate limit counters start from zero; old
  tag counters never expire, `DeleteMatch("_tag:*")` removes them.
- `backend/redis` requires go-redis `v9.18.0`, which fixes GO-2025-3540; before, `v9.0.0`.
- `backend/redis`: a `Bus` that fails to read a message drops all local copies, as after a
  reconnect, since messages may have been lost.

### Fixed

- With every `MaxConcurrentLoads` slot taken, a call answers the stale result within any window of
  `Policy.Stale`, `IfSlow` included and whatever `IsFailure` says; it waited for a slot before.
- Concurrent calls for one key share a load that no longer runs with the `ctx` of the first
  caller: when that caller left, the others got its `context canceled`, and `httpcache` stored the
  page its cancelled handler wrote. The load is cancelled once every caller has left.
- `InvalidateTags` on a counter the backend evicted started it at 1, so entries stored with
  generation 1 were valid again; it starts at a random generation now, as a read does.
- `backend/memory`: a value over `MaxBytes` left the old value readable; the old entry is gone.
- `backend/nearcache`: a read whose key another process changed while the reply was on its way kept
  the old value for `LocalTTL`; it keeps no copy now.
- `LockOnMiss` with `Negative`: a process that waited for the lock loaded again instead of taking the
  "not found" the holder stored.
- `Reload` stored a "not found" when `Negative.If` accepted the error; any error leaves the stored
  result alone, as documented.
- `StaleIfError` hid a "not found" with the old result while it stored the "not found"; a "not
  found" is an answer now, not a failure.
- Type shapes count `msgpack` tags: with `codec/msgpack`, a renamed tag read old entries with empty
  fields. Types without `msgpack` tags keep their shape, so nothing reloads.
- `Options.CircuitBreaker` did not count calls that ended by their deadline, so it never opened for
  a hung backend. A call whose `ctx` ended before it started is not counted.
- `CircuitBreaker`: a failing call slower than a bucket wiped the newer counts, a late test call
  reopened a breaker another one closed, and the share of failures was checked only on failures.
- `backend/redis`: `ExtendLock` with less than a millisecond deleted the lock; it rounds up.
- `backend/redis`: after a failed read, `Bus` and `TrackingBus` dropped every copy each second
  while Redis was down, so a near cache served nothing; copies stay until the bus reconnects, and
  `FLUSHDB` no longer delays `TrackingBus` by a second.
- `httpcache`: a 5xx without `StaleIfError` made every waiting request render again; it is shared
  unless a header keeps it to one client. Headers named in `Connection` were stored, `Age` was set
  on responses rendered for the request, and `HEAD` had no `Content-Length`.
- `httpcache`: `Policy.Negative` never applied; `New` panics on it now.
- `httpcache`: a request whose `ctx` deadline passed while it waited for the render of another
  request got an empty `200`, as if its client were gone; it gets `503` now.
- `httpcache`: a stale page `StaleIfError` answers instead of a 5xx is marked
  `Cache-Status: gashe; fwd=stale; fwd-status=<status>`, as RFC 9211 asks, not `hit`, and has `Age`.

### Security

- `httpcache` no longer caches requests with a `Cookie` header, as it does not for
  `Authorization`: a page that depends on the session but sends no `Cache-Control: private` was
  stored under its URL and served to everyone (web cache deception). `Vary.Cookies` lists the
  cookies a page depends on, and `Vary.User` stores pages per user; with either, such requests are
  cached.

## [0.4.0] - 2026-11-03

Released as `v0.4.0` and `compress/zstd/v0.1.0`, the first version of that module.
`backend/redis` and `codec/msgpack` have no changes and stay at `v0.3.0`.

### Added

- `benchmarks/`: benchmarks of every codec and compressor, run with `just bench`; the README has the
  results and the recommended choice.
- `Options.Compressor` and the `Compressor` interface: compression is set apart from the format of
  `Options.Codec` and works with any codec.
- `compress/zstd` (module): a `Compressor` based on Zstandard; it stores about what gzip does and is
  faster.
- `compress/gzip`: a `Compressor` based on gzip, in the root module; it reuses its writers, so a
  small value costs a fraction of what `Gzip` did.
- `For` and `Cached[V]`: a `Policy` bound to a `Cache` and checked once, with `GetOrLoad`,
  `GetOrLoadMany` and `Reload`, `Set` that stores a value you already have as a load would, and
  `Delete` that applies `Policy.Prefix`.
- `Cache.WithLock`: runs a function under a lock and extends the lock while it runs; a lost lock
  cancels the function's `ctx`.
- `Guard`: runs one call through `CircuitBreaker` and reports its result.
- `RateLimitError` with `RetryAfter`: over the limit `RateLimit` returns a `*RateLimitError`, still
  `ErrRateLimited` for `errors.Is`, that says how long until a call passes again, for a
  `Retry-After` header.

### Changed

- `Gzip` is removed: `Codec: gashe.Gzip{Inner: c}` becomes `Codec: c, Compressor: gzip.New()`.
- `Raw` moves to `codec/raw`: `gashe.Raw{}` becomes `raw.Codec{}`. The core keeps the interfaces and
  the defaults, `JSON` and no compression; the alternatives are packages.

## [0.3.0] - 2026-11-03

Released as `v0.3.0`, `backend/redis/v0.3.0` and `codec/msgpack/v0.3.0`. The API changed in ways
that break code written for earlier versions, so they are retracted: `v0.1.0` and `v0.2.0` of the
root, `backend/redis/v0.1.0` and `backend/redis/v0.2.0`, and `codec/msgpack/v0.1.0`.
`go get` no longer picks them, and `go list -m -u` points to `v0.3.0`.

### Added

- `examples/`: a runnable program for every feature, in a module of its own; the README links each
  strategy to its example.

### Changed


- The keys the cache keeps for itself start with `_` and live under `Options.Prefix`: `_tag:`,
  `_load:`, `_refresh:`, `_rate:sliding:`, `_rate:fixed:`. `Options.Namespace` is removed; keys of
  yours must not start with `_`. After the upgrade, tagged entries reload once and rate limit
  counters start from zero.
- `Options.Prefix` replaces `WithPrefix`: `gashe.New(b, gashe.Options{Prefix: "orders:"})` puts every
  key of the cache under the prefix, and `Policy.Prefix` adds up after it. `WithPrefix` is removed.
- `Options.CircuitBreaker` replaces `WithBreaker`: `gashe.New(b, gashe.Options{CircuitBreaker: &opts})`
  puts the backend behind a circuit breaker. `WithBreaker` is removed, and `BreakerOptions` is
  renamed `CircuitBreakerOptions`.
- `Policy.TTL` takes a `time.Duration`, a string such as `"1h"` or a `func(V) time.Duration`, and
  `Policy.TTLFunc` is removed: write `TTL: func(v V) time.Duration {...}`. Another type returns
  `ErrInvalidOptions`.
- `Incr` is renamed `IncrBy`, like the Redis command it matches, on `Cache` and `Backend`; a backend
  of yours renames its method too.
- `backend/redis` and `codec/msgpack` require gashe `v0.3.0`. The tests of `backend/redis` against a real server moved into the module
  and run with `just integration`; the separate integration module and testcontainers are gone.
  Valkey 8 joins Redis 7.0 in the servers every release is tested on.

## [0.2.0] - 2026-11-02

Released as `v0.2.0` and `backend/redis/v0.2.0`; `codec/msgpack` stays at `v0.1.0`.

### Added

- `nearcache.Bus` and `nearcache.Options.Bus`: a near cache tells the near caches of other
  processes which keys it changed, so they drop their copies at once instead of after `LocalTTL`.
  `InvalidateTags` becomes instant on every process too.
- `backend/redis`: `NewBus`, a `nearcache.Bus` over Redis pub/sub. It drops all local copies after
  every reconnect, since messages may have been lost, and works on a Cluster.
- `httpcache`: a middleware that caches whole HTTP responses of a handler through `GetOrLoad`,
  with `ETag`, `304` and `Cache-Status` (RFC 9211), and serves the last good page on a 5xx with
  `StaleIfError`.
- `Policy.StaleIfSlow` and `ResultStaleIfSlow`: when the loader is slower than this, the call
  answers with the stale result and the load finishes in the background, one per key.
- `WithBreaker`: a backend wrapper that stops calling a failing backend for a while, so a cache
  that is down costs no timeout on every call.
- Entries keep the shape of their type (`t`): after a deploy that changes the type, old entries are
  misses instead of decoding into empty fields. Entries from 0.1.0 are read as they are.

### Changed

- The keys the cache keeps for itself live under `Options.Namespace`, `gashe:` by default:
  `gashe:load:<key>` and `gashe:refresh:<key>` instead of `<key>:load` and `<key>:refresh`,
  `gashe:rate:sliding:` and `gashe:rate:fixed:` instead of `srl:` and `rate_limit:`. After the
  upgrade, rate limit counters start from zero; tag counters keep their keys unless you set a
  namespace.
- `Options.OnResult` receives an `Outcome` with the key, the `Result` and how long the loader ran:
  `func(ctx context.Context, o gashe.Outcome)` instead of `func(ctx, key string, r Result)`.
- Results whose types do not explain them are named in signatures, as `SetIfAbsent` returning
  `(stored bool, err error)`; calls do not change.
- `nearcache.Backend.Close` stops listening to the `Bus` and waits for it. It still closes neither
  backend.
- The documentation is one shorter README; development notes moved to `CONTRIBUTING.md`, and the
  Russian translation was removed.
- `REFERENCE.md` was removed: the API reference is pkg.go.dev, built from the doc comments. The
  glossary and the images moved to `docs/`.

### Fixed

- A tag counter that the backend evicted read as generation 0, which made entries invalidated before
  the eviction valid again. A missing counter now starts at a random generation.

## [0.1.0] - 2026-11-02

The first release.

### Added

- `gashe.Cache` over a byte `Backend`, with `Options` for the codec, `OnError`, `OnResult` and
  `LockPollInterval`.
- Caching with `GetOrLoad`, `GetOrLoadMany` and `Reload`, configured by `Policy`: `TTL`, `TTLFunc`,
  `Jitter`, `StaleWhileRevalidate`, `StaleIfError`, `StaleIf`, `LockOnMiss`, `Negative`, `Tags`,
  `CacheIf` and `Prefix`. Concurrent calls in one process share one load.
- Tag invalidation with `InvalidateTags`, one write per tag.
- Distributed locks: `Lock`, `TryLock`, `IsLocked`, `Lease.Extend`, `Lease.Unlock`.
- Rate limits: `RateLimit` with `SlidingWindow` (`PerSecond`, `PerMinute`, `PerHour`) and
  `FixedWindow` with a ban.
- An in-process circuit breaker: `CircuitBreaker` with `BreakerOptions`.
- The basic API: `Get`, `Set`, `SetIfAbsent`, `SetIfPresent`, `GetMany`, `SetMany`, `Keys`, `All`,
  `Delete`, `DeleteMany`, `DeleteMatch`, `Exists`, `Incr`, `Expire`, `TTL`, `Ping`.
- `Cache.Shutdown`, which waits for background reloads.
- Codecs `JSON`, `Gzip` and `Raw`; `WithPrefix` for a shared Redis.
- `backend/memory`: an LRU in process memory bounded by `MaxEntries` and `MaxBytes`.
- `backend/nearcache`: a memory cache in front of a shared backend.
- `backendtest`: the contract suite for your own backends.
- `backend/redis` (module): a backend over go-redis v9, for a single node, Sentinel or Cluster;
  tested on Redis 7, Valkey 8, Dragonfly and KeyDB.
- `codec/msgpack` (module): a MessagePack codec.

[Unreleased]: https://github.com/eugeneliukindev/gashe/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.4.0
[0.3.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.3.0
[0.2.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.2.0
[0.1.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.1.0
