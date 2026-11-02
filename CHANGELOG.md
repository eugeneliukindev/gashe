# Changelog

Notable changes of every release, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/): while the version is `0.x`, a minor release may change
the API. The root module and `backend/redis` and `codec/msgpack` are released with their own tags
(`v0.2.0`, `backend/redis/v0.2.0`, `codec/msgpack/v0.2.0`); a section names the module it is about.

## [Unreleased]

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

[Unreleased]: https://github.com/eugeneliukindev/gashe/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.3.0
[0.2.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.2.0
[0.1.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.1.0
