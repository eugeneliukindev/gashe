# gashe

A caching library: it stores results of expensive calls and protects callers from slow or failing dependencies.

## Language

### Storage

**Cache**:
The one object a program works with; it stores values in a backend and runs caching, locks, rate limits and circuit breakers on top of it.

**Backend**:
A place where entries live, such as process memory or Redis.
_Avoid_: storage, driver, store

**Entry**:
One key with its stored value and optional expiry.
_Avoid_: item, record

**Key**:
The name an entry is stored under, built by the caller.

**Prefix**:
A fixed leading part added to a key so that entries of different kinds never collide.

**TTL**:
How long an entry lives before it expires, or, for a cached result, how long it is fresh.
_Avoid_: expire, timeout

**Codec**:
The rule that turns a value into stored bytes and back.
_Avoid_: serializer, pickler, marshaler

**Hit**:
A lookup that finds a live entry.

**Miss**:
A lookup that finds no live entry.

### Caching

**Loader**:
The caller's function that produces the real value when the cache cannot answer.
_Avoid_: fn, callback, getter

**Fresh**:
A stored result that is answered without calling the loader.

**Stale result**:
A stored result past its TTL but still kept; answered only while it is revalidated or when the loader fails.
_Avoid_: old result, outdated

**Stale-while-revalidate**:
The time after TTL in which a stale result is answered at once while one caller reloads it in the background.
_Avoid_: refresh window, early expiration

**Stale-if-error**:
The time after TTL in which a stale result answers a loader failure.
_Avoid_: fallback window, failover, soft expiration

**Lock on miss**:
A lock taken only when a key is missing, so that one process loads it while the others wait for its result.
_Avoid_: miss lock

**Reload**:
Replacing a stored result with a new loader result before it expires.
_Avoid_: refresh, recompute, revalidate

**Batch load**:
One loader call for all keys of a list that the cache could not answer.
_Avoid_: bulk load, multi-load

**Negative answer**:
A cached "there is no such data" result, kept for a short while so lookups of missing data do not reach the source.
_Avoid_: tombstone, null cache

**Jitter**:
A random spread of TTLs, so that entries stored together do not expire together.

**Tag**:
A name given to entries so that all of them can be invalidated together.
_Avoid_: group

**Near cache**:
A small cache in process memory in front of a shared backend.
_Avoid_: L1, local cache, client-side cache

**Bus**:
The channel through which near caches of different processes tell each other which keys changed.
_Avoid_: backplane, broadcast

**Cache rule**:
A rule that decides whether a loader result is worth storing.
_Avoid_: condition, filter, predicate

### Protection

**Lock**:
A claim on a key that lets one caller across all processes do the work while others wait or give up.
_Avoid_: mutex, semaphore

**Lease**:
A lock held by one caller, which it can extend or release.

**Rate limit**:
The most calls a key may make in a span of time.
_Avoid_: throttle, quota

**Sliding window**:
A rate limit over any span of the given length ending now.

**Fixed window**:
A rate limit over spans that start with the first call.

**Ban**:
The time a key stays limited after it first goes over a fixed window.

**Circuit breaker**:
A guard that stops calls to a dependency for a while once too many of them fail.
_Avoid_: fuse

**Open breaker**:
A circuit breaker that refuses every call.

**Half-open call**:
A call let through after a breaker was open, to test whether the dependency works again; its outcome closes or reopens the breaker.
_Avoid_: trial call, probe
