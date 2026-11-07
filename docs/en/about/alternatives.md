# Alternatives and comparisons { #alternatives-and-comparisons }

gashe was built for one case: a Go service that runs as many processes over a shared Redis and must keep
answering while its dependencies are slow or down. Many good Go caches solve other cases better.
This page says what each one is, what it does well, what it lacks next to gashe and when to choose
it instead.

- **Checked on:** 2026-11-06, from the README, `go.mod` and source of the default branch of each
  project, linked in each section.
- **Unknown:** a `?` in the [table](#comparison-table) means the docs did not say and it was not
  checked in the code.
- **Corrections:** projects change; an issue on [GitHub](https://github.com/eugeneliukindev/gashe/issues)
  fixes this page.

## In-process caches { #in-process-caches }

They keep Go values in the memory of one process: fast, no network, no encoding, but every process has
its own copy and its own misses.

### Ristretto { #ristretto }

[dgraph-io/ristretto](https://github.com/dgraph-io/ristretto), used by Badger and Dgraph.

- **Is:** an in-process cache with a cost per entry and a TTL (`SetWithTTL`).
- **Does well:** high hit ratio with TinyLFU admission and SampledLFU eviction, high throughput
  under many goroutines, metrics.
- **Lacks next to gashe:** no loader, so no stampede protection or stale data; not shared between
  processes ("Is Ristretto distributed? No"); a `Set` of a new item may be dropped by design.
- **Choose it when:** one process, a hot read path, and the hit ratio matters more than features.

### Otter { #otter }

[maypok86/otter](https://github.com/maypok86/otter), [user guide](https://maypok86.github.io/otter/).

- **Is:** an in-process cache, modelled on Java's Caffeine.
- **Does well:** adaptive W-TinyLFU eviction, low memory overhead, a loader that deduplicates
  concurrent loads, asynchronous refresh of a stale entry, statistics, saving to a file; no
  third-party dependencies outside tests.
- **Lacks next to gashe:** one process only; a failed refresh keeps the old value, but nothing
  answers stale data past its expiry; a loader that returns [`ErrNotFound`](../reference/gashe.md#ErrNotFound) removes the entry instead of
  caching the miss; no tags, locks, limits or breakers. Needs Go 1.24, and supports the two newest Go
  releases.
- **Choose it when:** one process and you want the best in-memory cache with loading and refresh.

### sturdyc { #sturdyc }

[viccon/sturdyc](https://github.com/viccon/sturdyc).

- **Is:** an in-process cache built to shield data sources, with an optional distributed storage
  behind it.
- **Does well:** stampede protection, early refreshes in the background, caching of missing
  records, batch fetches cached per record, refresh coalescing of many ids into one batch call.
- **Lacks next to gashe:** the distributed storage is an interface you implement, and you set its
  TTL and eviction yourself; no lock that makes one process load a cold key for all processes; no tags,
  locks, rate limits, breakers or HTTP middleware.
- **Choose it when:** the service mostly calls batch APIs and needs few calls to them, with memory
  as the main cache.

### BigCache and FreeCache { #bigcache-and-freecache }

[allegro/bigcache](https://github.com/allegro/bigcache), [coocood/freecache](https://github.com/coocood/freecache).

- **Are:** in-process stores of bytes, built so that millions of entries cost the garbage collector
  almost nothing.
- **Do well:** very large caches with short GC pauses. BigCache grows when full unless
  `HardMaxCacheSize` is set, then overwrites the oldest entries; FreeCache preallocates its memory
  and evicts nearly in LRU order.
- **Lack next to gashe:** values are bytes, so you encode them; no loader, stale data or
  invalidation by tag; one process. BigCache has one TTL for the whole cache (`LifeWindow`) and does
  not handle hash collisions: a new item overwrites one whose hash collides.
- **Choose them when:** you hold gigabytes in one process and GC pauses are the problem.

### Simple maps with expiry { #simple-maps-with-expiry }

| Library | Is | Choose it when |
|---|---|---|
| [patrickmn/go-cache](https://github.com/patrickmn/go-cache) | a thread-safe `map[string]interface{}` with expiry | a small cache in one process, no size limit needed |
| [hashicorp/golang-lru](https://github.com/hashicorp/golang-lru) | a fixed-size LRU, with an `expirable` variant | a bounded map in one process |
| [jellydator/ttlcache](https://github.com/jellydator/ttlcache) | a generic cache with TTL, a `Loader` and events | one process, a loader is enough |

None of them is shared between processes or answers stale data while a source fails.

## Shared and distributed caches { #shared-and-distributed-caches }

### go-redis/cache { #go-rediscache }

[go-redis/cache](https://github.com/go-redis/cache).

- **Is:** a cache over Redis, with an optional local in-process cache (TinyLFU) in front.
- **Does well:** small and simple; `Once` loads a missing key once per process (singleflight);
  MessagePack encoding and compression; `SetNX` and `SetXX`.
- **Lacks next to gashe:** the local cache of other processes is not told about a change, so it
  stays stale until its TTL; no stale data, tags, locks for loads across processes, rate limits or breakers.
- **Choose it when:** you want the shortest path from go-redis to typed values in Redis.

### gocache { #gocache }

[eko/gocache](https://github.com/eko/gocache).

- **Is:** one cache API over many stores: memory (BigCache, Ristretto, go-cache, FreeCache), Redis,
  Redis Cluster, rueidis, Valkey, Memcache, NATS, Hazelcast and more.
- **Does well:** the widest choice of stores; a chain cache (memory, then Redis) that fills upper
  stores from lower ones; a loadable cache with singleflight; tags; Prometheus metrics;
  `SetIfNotExists` for simple locks.
- **Lacks next to gashe:** stale data, negative caching and loads coordinated across processes are
  not described; the memory layer of a chain is not invalidated in other processes; tag entries can be evicted
  by the store, after which invalidation removes nothing (its README warns of this); no rate
  limits, breakers or HTTP middleware.
- **Choose it when:** you need a store gashe has no backend for, such as Memcache or Hazelcast, or
  one API over several stores.

### rueidis { #rueidis }

[redis/rueidis](https://github.com/redis/rueidis), with
[rueidisaside](https://github.com/redis/rueidis/blob/main/rueidisaside/README.md),
[rueidislock](https://github.com/redis/rueidis/tree/main/rueidislock) and
[rueidislimiter](https://github.com/redis/rueidis/tree/main/rueidislimiter).

- **Is:** a Redis client, not a cache library, with server-assisted client-side caching and helper
  packages.
- **Does well:** a local copy that Redis invalidates itself, auto pipelining; `rueidisaside` loads a
  missed key once by locking it in Redis, and others wait for the notification; distributed locks
  and rate limits (GCRA and fixed window).
- **Lacks next to gashe:** no stale data, tags or breakers; caching a miss is done by hand (the
  example stores a `"_nil_"` value); it replaces go-redis rather than working with it.
- **Choose it when:** you are free to use rueidis as the client and want Redis-tracked local copies
  above all.

### groupcache { #groupcache }

[golang/groupcache](https://github.com/golang/groupcache), and the fork
[mailgun/groupcache](https://github.com/mailgun/groupcache).

- **Is:** a distributed cache formed by the processes themselves: each key has an owner peer, found
  by consistent hashing; no Redis or memcached.
- **Does well:** one load per key across all peers, then shared with every caller; hot keys mirrored
  to other peers; no servers to run.
- **Lacks next to gashe:** values never change: no expiry and no delete in the original; the
  mailgun fork adds expiry and `Remove`, sent to every peer on a best-effort basis. No stale data,
  tags, locks, limits or breakers; peers need to find each other.
- **Choose it when:** values are immutable for a key (files, rendered blobs by hash) and you do not
  want to run Redis.

## Comparison table { #comparison-table }

As of 2026-11-06. `yes`: built in; `partial`: see the note; `no`: not offered; `?`: not checked.

| | gashe | Ristretto | Otter | sturdyc | BigCache | FreeCache | go-redis/cache | gocache | rueidis | groupcache |
|---|---|---|---|---|---|---|---|---|---|---|
| Shared across processes (Redis) | yes | no | no | partial (1) | no | no | yes | yes | yes | partial (2) |
| Stampede protection, one process | yes | no | yes | yes | no | no | yes | yes | yes (3) | yes |
| Stampede protection, across processes | yes | no | no | no | no | no | no | no | yes (3) | yes |
| Stale-while-revalidate | yes | no | yes | yes | no | no | no | no | no | no |
| Stale-if-error | yes | no | partial (4) | partial (4) | no | no | no | no | no | no |
| Negative caching | yes | no | no | yes | no | no | ? | ? | partial (5) | no |
| Tags, invalidation | yes | no | no | no | no | no | no | yes | no | no |
| Near cache | yes | no | no | partial (1) | no | no | partial (6) | partial (6) | yes | partial (2) |
| Circuit breaker | yes | no | no | no | no | no | no | no | no | no |
| Rate limits | yes | no | no | no | no | no | no | no | yes | no |
| Locks | yes | no | no | no | no | no | no | partial (7) | yes | no |
| HTTP middleware | yes | no | no | no | no | no | no | no | no | no |
| Zero dependencies (8) | yes | no | yes | no | yes | no | no | no | no | no |
| Eviction in memory | LRU | TinyLFU + SampledLFU | W-TinyLFU | by recency | oldest first | nearly LRU | TinyLFU | per store | LRU | LRU |

1. sturdyc reads and writes a `DistributedStorage` you implement; it brings no Redis code.
2. groupcache shares values among its own peers, not through Redis; hot keys are copied to peers.
3. Through `rueidisaside`: the first miss locks the key in Redis, the others wait.
4. A failed refresh keeps the old value until it expires; nothing answers past the expiry.
5. Redis nils are cached client-side; caching a miss of your source is done by hand.
6. A local cache in front of Redis; other processes are not told of a change and keep their copy until
   its TTL.
7. `SetIfNotExists`, which its README calls a building block for simple locks; no unlock or extend.
8. Third-party imports of the main module, outside tests and examples. gashe: the core module;
   `backend/redis` needs go-redis.

## Where gashe is weaker { #where-gashe-is-weaker }

| Point | Better choice |
|---|---|
| Every value goes through a codec, even in [`backend/memory`](../concepts/backends.md#backendmemory), so a hit costs a decode | Otter, Ristretto: Go values, no encoding |
| `backend/memory` is a plain LRU bounded by entries and bytes | Otter, Ristretto: higher hit ratio |
| Redis is the only shared backend it ships; others need [your own backend](../concepts/backends.md#backendtestrun) | gocache: many stores |
| Version `0.x`: a minor release may change the API, see the [version policy](version-policy.md) | the libraries above with a `v1` or later |
| Its [known limits](../internals/storage.md#known-limits): the race of `Delete`, advisory locks, clocks in sync | - |

## When gashe fits { #when-gashe-fits }

Choose gashe when most of these are true:

- the service runs as several processes over Redis or Valkey;
- a cold or expired key must load once for all processes: [`Stampede.LockFor`](../concepts/stampedes.md);
- a stale result is better than an error while the source is slow or down: [stale data](../concepts/stale.md#staleiferror-staleifslow);
- one change drops many entries: [tags](../concepts/invalidation.md#tags);
- the same cache should also give [locks](../concepts/locks.md#locks), [rate limits](../concepts/rate-limits.md#cratelimit)
  and [circuit breakers](../concepts/breakers.md#gasheguard).
