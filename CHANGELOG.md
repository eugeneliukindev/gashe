# Changelog

Notable changes of every release, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/): while the version is `0.x`, a minor release may change
the API. The root module and `backend/redis` and `codec/msgpack` are released with their own tags
(`v0.2.0`, `backend/redis/v0.2.0`, `codec/msgpack/v0.2.0`); a section names the module it is about.

## [0.2.0] - Unreleased

### Added

- `nearcache.Bus` and `nearcache.Options.Bus`: a near cache tells the near caches of other
  processes which keys it changed, so they drop their copies at once instead of after `LocalTTL`.
  `InvalidateTags` becomes instant on every process too.
- `backend/redis`: `NewBus`, a `nearcache.Bus` over Redis pub/sub. It drops all local copies after
  every reconnect, since messages may have been lost, and works on a Cluster.
- `httpcache`: a middleware that caches whole HTTP responses of a handler through `GetOrLoad`,
  with `ETag` and `304`, and serves the last good page on a 5xx with `StaleIfError`.

### Changed

- `nearcache.Backend.Close` stops listening to the `Bus` and waits for it. It still closes neither
  backend.
- The documentation is one shorter README; development notes moved to `CONTRIBUTING.md`, and the
  Russian translation was removed.

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

[0.2.0]: https://github.com/eugeneliukindev/gashe/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/eugeneliukindev/gashe/releases/tag/v0.1.0
