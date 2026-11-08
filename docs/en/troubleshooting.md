# Troubleshooting { #troubleshooting }

Find what you see, then the row whose "How to tell" matches; most causes show in [`Options.Hooks.OnResult`](reference/gashe.md#Options),
[`Options.Hooks.OnError`](reference/gashe.md#Options) or the `Cache-Status` header.

| Where to look | What it shows |
|---|---|
| [`Options.Hooks.OnResult`](reference/gashe.md#Options) | [`Outcome.Key`](reference/gashe.md#Outcome), the key with [`Policy.Prefix`](reference/gashe.md#Policy), and [`Outcome.Result`](reference/gashe.md#Outcome): `hit`, `miss`, `stale-while-revalidate`, `stale-if-error`, `stale-if-slow`, `negative-hit`, `fallback`; one process each |
| [`Options.Hooks.OnError`](reference/gashe.md#Options) | the errors the cache works around, each naming the operation and the key: `gashe: get "user:42": ...`, `gashe: decode "user:42": ...` |
| `Cache-Status` | how `httpcache` handled a request, see [below](#httpcache-does-not-cache-a-page) |

More: [monitoring](concepts/setup.md#onresult-onerror), [every error](errors.md#errors).

---

## Every request still goes to the database { #every-request-goes-to-the-database }

| Cause | How to tell | Fix |
|---|---|---|
| The `TTL` function returns a negative duration, or 0 without a stale window: nothing is stored | `miss` on every call of one key; check what the function returns, such as `time.Until` of a time already past | return a positive duration for results to keep |
| The `TTL` function returns 0 with a stale window | every call is past `TTL`: `stale-while-revalidate`, or `miss` within [`Stale.IfError.For`](reference/gashe.md#Stale) | return a positive duration |
| [`Store.If`](reference/gashe.md#Store) returns false | `miss` on every call; the result is returned, not stored | let the results to keep pass [`Store.If`](reference/gashe.md#Store) |
| The loader fails: its error is returned, not stored | the same error on every call, such as `sql.ErrNoRows` for an id that does not exist | [`Negative`](reference/gashe.md#Negative) for "not found"; [`Stale.IfError`](reference/gashe.md#Stale) for a failing source |
| The backend fails, and the policy has no [`Fallback`](reference/gashe.md#Fallback) | [`OnError`](reference/gashe.md#Hooks) gets `gashe: get ...` or `gashe: set ...`; `miss` on every call | [`Policy.Fallback`](reference/gashe.md#Policy), and [`Options.CircuitBreaker`](reference/gashe.md#Options) so calls do not wait for the backend |
| The key differs per call: a timestamp, a request id, a query in the order it was sent | every [`Outcome.Key`](reference/gashe.md#Outcome) is new | build the key from what the result depends on, nothing else |
| A rolling deploy changed the type: old and new processes overwrite each other's entries | `miss` on keys that were hits before the deploy, until the rollout ends | a new [`Policy.Prefix`](reference/gashe.md#Policy), such as `users:v2:` |
| Two call sites read one key as two types | `miss` on every call of the key, sometimes `gashe: decode` in [`OnError`](reference/gashe.md#Hooks) | a key or [`Policy.Prefix`](reference/gashe.md#Policy) per type |
| [`InvalidateTags`](reference/gashe.md#Cache.InvalidateTags) runs on almost every write, for a tag many entries share | `miss` right after a store; the entries are still in the backend | narrower tags: `user:42`, not `users` |
| A `backend/memory` per process, or a new one per request: memory is not shared | each process has its own `miss` for the same key; with a new backend per request, only `miss` | `backend/redis` shared by every process; create the backend and the [`Cache`](reference/gashe.md#Cache) once at startup |
| `backend/memory` evicts: [`MaxEntries`](reference/memory.md#Options) is 1000 unless set, [`MaxBytes`](reference/memory.md#Options) bounds the bytes | `miss` on keys stored a moment ago; [`memory.ErrTooLarge`](reference/memory.md#ErrTooLarge) in [`OnError`](reference/gashe.md#Hooks) for a value over [`MaxBytes`](reference/memory.md#Options) alone | a larger [`MaxEntries`](reference/memory.md#Options) or [`MaxBytes`](reference/memory.md#Options) |
| Redis evicts keys, with `maxmemory` | `evicted_keys` grows in `INFO stats` of Redis | more memory for Redis, or shorter TTLs |

More: [TTL](concepts/loading.md#ttl),
[Store.If](concepts/loading.md#cacheif-prefix),
[Fallback](concepts/outages.md#fallback),
[type changes](recipes/changing-types.md#during-a-rolling-deploy),
[backend/memory](concepts/backends.md#backendmemory),
[known limits](internals/storage.md#known-limits).

---

## A cold key makes all processes query the database at once { #a-cold-key-makes-all-processes-query-the-database }

| Cause | How to tell | Fix |
|---|---|---|
| No [`Stampede.LockFor`](reference/gashe.md#Stampede): calls within one process share a load, processes do not | one `miss` per process for the same key at the same moment | [`Stampede.LockFor`](reference/gashe.md#Stampede), longer than the slowest load |
| A `backend/memory` per process: the lock lives in the memory of each process | as above, with [`Stampede.LockFor`](reference/gashe.md#Stampede) set | `backend/redis`, so the lock is shared |
| [`Stampede.LockFor`](reference/gashe.md#Stampede) is shorter than the load: the lock expires and the next waiting process loads too | several `miss` for the key, each [`Outcome.Load`](reference/gashe.md#Outcome) longer than [`Stampede.LockFor`](reference/gashe.md#Stampede) | [`Stampede.LockFor`](reference/gashe.md#Stampede) longer than the slowest load |
| The backend fails: [`Stampede.LockFor`](reference/gashe.md#Stampede) cannot lock and every process loads | [`OnError`](reference/gashe.md#Hooks) gets `gashe: load lock ...` | a breaker in front of the database, [`gashe.Guard`](reference/gashe.md#Guard); [`Policy.Fallback`](reference/gashe.md#Policy) |
| The entry is expired, not missing: past `TTL` and [`Stale.WhileRevalidate`](reference/gashe.md#Stale), within [`Stale.IfError.For`](reference/gashe.md#Stale), every process reloads; [`Stampede.LockFor`](reference/gashe.md#Stampede) covers only a key with no usable entry | `miss` in every process while the backend still holds the entry | [`Stale.WhileRevalidate`](reference/gashe.md#Stale) or [`Stale.IfSlow`](reference/gashe.md#Stale): both reload under one lock for all processes |
| Many keys were stored together and expire together | `miss` peaks one `TTL` after a deploy or a warm-up | [`TTL.Jitter`](reference/gashe.md#TTL), such as 0.1 |

More: [Stampede.LockFor](concepts/stampedes.md#lockonmiss),
[Jitter](concepts/stampedes.md#jitter),
[stale data](concepts/stale.md#stalewhilerevalidate),
[breakers](concepts/breakers.md#gasheguard).

---

## Old data after a change { #old-data-after-a-change }

| Cause | How to tell | Fix |
|---|---|---|
| Nothing invalidates the entry: it lives its `TTL` | `hit` with the old result until `TTL` ends | [`Tags`](reference/gashe.md#Policy) and [`Cache.InvalidateTags`](reference/gashe.md#Cache.InvalidateTags) after the write, or [`Cached.Set`](reference/gashe.md#Cached[V].Set) or [`Reload`](reference/gashe.md#Reload) with the new data |
| [`InvalidateTags`](reference/gashe.md#Cache.InvalidateTags) runs before the write: a load in between stores the old data under the new generation | old data after an invalidation, now and then | call [`InvalidateTags`](reference/gashe.md#Cache.InvalidateTags) after the write, not before |
| A load that started before [`Cached.Set`](reference/gashe.md#Cached[V].Set) or [`Cached.Delete`](reference/gashe.md#Cached[V].Delete) stores its older result after it | now and then, under concurrent reads of the key | [`Tags`](reference/gashe.md#Policy): a load reads the counters before it calls the loader |
| [`Stale.WhileRevalidate`](reference/gashe.md#Stale): past `TTL` the old result answers while one process reloads | `stale-while-revalidate` | a shorter window, or invalidate after the write |
| [`Stale.IfError`](reference/gashe.md#Stale) or [`Stale.IfSlow`](reference/gashe.md#Stale): the loader failed or was slow | `stale-if-error` with `gashe: "<key>" answered with an old result: ...` in [`OnError`](reference/gashe.md#Hooks); `stale-if-slow` | fix the source; a shorter window, or [`Stale.IfError.IsFailure`](reference/gashe.md#Stale) to cover fewer errors |
| A near cache without a `Bus`: other processes keep their copy | old data in other processes for up to [`LocalTTL`](reference/nearcache.md#Options), the process that wrote sees the new one | `nearcache.Options.Bus` |
| A near cache with [`gasheredis.NewBus`](reference/redis.md#NewBus), and the change was made past gashe: `redis-cli`, another service | the new value is in Redis, processes answer the old one for up to [`LocalTTL`](reference/nearcache.md#Options) | [`gasheredis.NewTrackingBus`](reference/redis.md#NewTrackingBus), not on a Cluster |
| [`Negative.TTL`](reference/gashe.md#Negative) keeps "not found" for data created meanwhile | `negative-hit` for the new id | a short [`Negative.TTL`](reference/gashe.md#Negative), or [`Cached.Delete`](reference/gashe.md#Cached[V].Delete) of the key when you create the data |
| The backend fails: [`Fallback`](reference/gashe.md#Fallback) answers what it kept, and no invalidation reaches it | `fallback` | a shorter [`Fallback.MaxTTL`](reference/gashe.md#Fallback) for data that changes often |
| The write goes through a [`Cache`](reference/gashe.md#Cache) with another [`Options.Prefix`](reference/gashe.md#Options), or [`Cached.Set`](reference/gashe.md#Cached[V].Set) through a policy with another [`Policy.Prefix`](reference/gashe.md#Policy) | the stored key in `redis-cli --scan` differs from [`Outcome.Key`](reference/gashe.md#Outcome) of the readers | the same prefixes on both sides; tags are under [`Options.Prefix`](reference/gashe.md#Options), not [`Policy.Prefix`](reference/gashe.md#Policy) |

More: [tags](concepts/invalidation.md#tags),
[Cached.Set](concepts/invalidation.md#cachedset),
[stale data](concepts/stale.md#staleiferror-staleifslow),
[which bus](concepts/backends.md#which-bus),
[Negative](concepts/negative.md#negative-cache-not-found).

---

## Requests hang or are slow while Redis is down { #requests-hang-while-redis-is-down }

| Cause | How to tell | Fix |
|---|---|---|
| go-redis retries: `MaxRetries` 3, `DialerRetries` 5 and `DialTimeout` 5s by default, so `ReadTimeout` does not bound a call | a call takes seconds; with `ReadTimeout` 100ms and the defaults, about 2.1s against a stopped Redis | `MaxRetries: -1`, a short `DialTimeout`, fewer `DialerRetries` |
| go-redis ignores the `ctx` deadline for reads and writes unless `ContextTimeoutEnabled` is set | calls outlive the deadline of the request | `ContextTimeoutEnabled: true` in the go-redis options |
| No [`Options.CircuitBreaker`](reference/gashe.md#Options): every call waits for go-redis to give up | every request is slow for the whole outage | [`Options.CircuitBreaker`](reference/gashe.md#Options) |
| Under load the breaker opens about one [`Window`](reference/gashe.md#CircuitBreaker) after the outage starts: the successes from before it must leave the window first | requests are slow for about [`Window`](reference/gashe.md#CircuitBreaker), then fast with [`ErrCircuitOpen`](reference/gashe.md#variables) in [`OnError`](reference/gashe.md#Hooks) | a shorter [`Window`](reference/gashe.md#CircuitBreaker) |
| Every call loads, and [`Stampede.MaxLoads`](reference/gashe.md#Stampede) queues the loads as long as `ctx` allows | calls pile up; without a deadline they wait however long the queue | give requests a deadline, `context.WithTimeout` or `http.TimeoutHandler`; [`Policy.Fallback`](reference/gashe.md#Policy) so a key loads once per process |

More: [CircuitBreaker](concepts/outages.md#circuitbreaker),
[backend/redis](concepts/backends.md#backendredis),
[Stampede.MaxLoads](concepts/stampedes.md#maxconcurrentloads).

---

## `ErrOverloaded`, or `503` from `httpcache` { #erroverloaded-or-503 }

| Cause | How to tell | Fix |
|---|---|---|
| Every slot of [`Stampede.MaxLoads`](reference/gashe.md#Stampede) stayed taken until `ctx` ended | `errors.Is(err, gashe.ErrOverloaded)`; it wraps `ctx.Err()`, so `context.DeadlineExceeded` holds too | a higher [`Stampede.MaxLoads`](reference/gashe.md#Stampede), a faster loader, a longer deadline; [`Stale.IfError`](reference/gashe.md#Stale), so a stale result answers instead of waiting |
| `httpcache`: the `ctx` deadline passed while the request waited for a load, with or without the limit: a [`Stampede.LockFor`](reference/gashe.md#Stampede) lock, a render of another request | `503` with `Cache-Status: gashe; fwd=uri-miss` | a deadline longer than a render, or a faster handler |
| The slots belong to a kind of data, its type and [`Policy.Prefix`](reference/gashe.md#Policy), in one [`*gashe.Cache`](reference/gashe.md#Cache): every `httpcache` middleware on one [`Cache`](reference/gashe.md#Cache) with the same [`Policy.Prefix`](reference/gashe.md#Policy) shares one limit, and its size is the value of the first load of the kind | a route gets `503` while another route renders | a [`Policy.Prefix`](reference/gashe.md#Policy) per route, and the same [`Stampede.MaxLoads`](reference/gashe.md#Stampede) everywhere for one kind |

The limit counts per [`*gashe.Cache`](reference/gashe.md#Cache) and per process: two caches in one process have separate slots,
and 10 processes let up to 10 times as many loads reach the source.

More: [Stampede.MaxLoads](concepts/stampedes.md#maxconcurrentloads),
[503 in httpcache](concepts/httpcache.md#over-maxconcurrentloads-a-request-gets-503).

---

## `ErrLockLost` from `WithLock` { #errlocklost-from-withlock }

| Cause | How to tell | Fix |
|---|---|---|
| An extension came too late: a pause (GC, a frozen VM) or a slow backend kept it from running before the TTL ran out | `errors.Is(err, gashe.ErrLockLost)`; the error reads `gashe: extend lock "<key>": gashe: lock lost` | a longer TTL; [`WithLock`](reference/gashe.md#Cache.WithLock) extends every third of it |
| The lock key was deleted or overwritten: [`Cache.Delete`](reference/gashe.md#Cache.Delete), [`Cache.DeleteMatch`](reference/gashe.md#Cache.DeleteMatch), [`Cache.Set`](reference/gashe.md#Cache.Set) on the same key, `FLUSHDB`, Redis eviction | as above, at no particular time | lock keys of their own, such as `lock:reindex`, out of every [`DeleteMatch`](reference/gashe.md#Cache.DeleteMatch) pattern |
| You expect [`ErrLockLost`](reference/gashe.md#variables), and get `context.Canceled`: the function returned `ctx.Err()` | `errors.Is(err, context.Canceled)` after a lost lock | return `context.Cause(ctx)`, or nil, when `ctx` ends |
| You expect [`ErrLockLost`](reference/gashe.md#variables), and get a backend error: an extension failed with it, and it follows the same rule | `gashe: extend lock "<key>": gashe/redis: ...` | treat it as a lost lock too |
| The function returned an error of its own | [`WithLock`](reference/gashe.md#Cache.WithLock) returns that error | wrap `context.Cause(ctx)` into it, so `errors.Is` finds [`ErrLockLost`](reference/gashe.md#variables) |

More: [WithLock](concepts/locks.md#cwithlock).

---

## `ErrRateLimited` too early, or never { #errratelimited-too-early-or-never }

| Cause | How to tell | Fix |
|---|---|---|
| Too early: [`SlidingWindow`](reference/gashe.md#SlidingWindow) weighs the last span as if its calls were spread evenly, so calls bunched at its start still count | refused below `Calls` real calls in the last `Per` | [`FixedWindow`](reference/gashe.md#FixedWindow) for an exact count per window, or a higher `Calls` |
| Too early: [`FixedWindow`](reference/gashe.md#FixedWindow) blocks the key for [`BanFor`](reference/gashe.md#FixedWindow), 0 means `Per`, after the first call over the limit | refused after the calls stopped; [`RetryAfter`](reference/gashe.md#RateLimitError) is what is left of the ban | a shorter [`BanFor`](reference/gashe.md#FixedWindow) |
| Too early: the key is shared by more callers than you meant, such as one key for the whole API, or the IP of your proxy for every client behind it; every process counts into it | many callers refused at once | a key per what you limit: `"api-key:"+key`, the client IP your proxy sets |
| Too early: [`RateLimit`](reference/gashe.md#Cache.RateLimit) runs more than once per request, in a middleware and a handler, or on retries | the counter grows faster than requests | call it once, before the work it guards |
| Too many pass: [`FixedWindow`](reference/gashe.md#FixedWindow) lets up to twice `Calls` through at the border of two windows | bursts around the end of a window | [`SlidingWindow`](reference/gashe.md#SlidingWindow) |
| Never: the key differs per call, such as `r.RemoteAddr`, which holds the port too | a new `__rate:` key per call in `redis-cli --scan` | `net.SplitHostPort` of `r.RemoteAddr`, or another stable id |
| Never: the backend fails or [`Options.CircuitBreaker`](reference/gashe.md#Options) is open, and [`RateLimit`](reference/gashe.md#Cache.RateLimit) lets calls through | [`OnError`](reference/gashe.md#Hooks) gets `gashe: rate limit ...` | fix the backend; a stricter limit in front, if the work must never run unlimited |
| Never: a `backend/memory` per process, so each counts alone | with 10 processes, up to 10 times `Calls` pass | `backend/redis` |
| Never: the limit is invalid, so [`RateLimit`](reference/gashe.md#Cache.RateLimit) returns [`ErrInvalidOptions`](reference/gashe.md#variables), and code that checks only [`ErrRateLimited`](reference/gashe.md#variables) lets the call through | `errors.Is(err, gashe.ErrInvalidOptions)` | positive `Calls` and `Per`; treat every error of [`RateLimit`](reference/gashe.md#Cache.RateLimit) |

More: [RateLimit](concepts/rate-limits.md#cratelimit).

---

## `httpcache` does not cache a page { #httpcache-does-not-cache-a-page }

| Cause | How to tell | Fix |
|---|---|---|
| The method is not `GET` or `HEAD` | `Cache-Status: gashe; fwd=method` | none: other methods are not cached |
| The request has `Range` | `gashe; fwd=request` | none: `Range` requests go past the cache |
| The request has `Authorization`, and [`Vary.User`](reference/httpcache.md#Vary) is nil | `gashe; fwd=request` | [`Vary.User`](reference/httpcache.md#Vary), if the page depends on who asks |
| The request has `Cookie`, and [`Vary.Cookies`](reference/httpcache.md#Vary) and [`Vary.User`](reference/httpcache.md#Vary) are nil | `gashe; fwd=request` | [`Vary.Cookies`](reference/httpcache.md#Vary) with the cookies the page depends on, or [`Vary.User`](reference/httpcache.md#Vary) |
| The response has `Set-Cookie`, such as from a session middleware | `gashe; fwd=uri-miss` | keep cookies off the cached routes |
| The response has a [`Vary`](reference/httpcache.md#Vary) that [`Options.Vary`](reference/httpcache.md#Options) does not cover, such as `Vary: Accept-Encoding` of a compression middleware, or `Vary: *` | `gashe; fwd=uri-miss` | the header in [`Vary.Headers`](reference/httpcache.md#Vary); `Vary: *` is never stored |
| The response has `Cache-Control` `no-store`, `private` or `no-cache` | `gashe; fwd=uri-miss` | drop the directive on pages to share |
| The body is over [`MaxBodyBytes`](reference/httpcache.md#Options), 1 MB unless set | `gashe; fwd=uri-miss` | a larger [`MaxBodyBytes`](reference/httpcache.md#Options) |
| The status is not `200` with a nil [`Store.If`](reference/gashe.md#Store), or [`Store.If`](reference/gashe.md#Store) returns false; a 5xx is never stored | `gashe; fwd=uri-miss`; within [`Stale.IfError.For`](reference/gashe.md#Stale) a 5xx gets the last page, `gashe; fwd=stale; fwd-status=500` | a [`Store.If`](reference/gashe.md#Store) that stores the statuses you want, such as `404` |
| The policy is invalid, such as no `TTL` and no stale window: every request runs the handler, and [`OnError`](reference/gashe.md#Hooks) gets nothing | `gashe; fwd=uri-miss` on every request | a valid [`Policy`](reference/gashe.md#Policy): check it with [`gashe.For`](reference/gashe.md#For) at startup, which panics on it |
| The key differs per request: a random query parameter, with a nil [`Vary.Query`](reference/httpcache.md#Vary) | `gashe; fwd=uri-miss; stored` on every request, never `hit` | [`Vary.Query`](reference/httpcache.md#Vary) with the parameters the page depends on |
| The backend fails: the page is rendered on every request | `gashe; fwd=uri-miss; stored` on every request; [`OnError`](reference/gashe.md#Hooks) gets `gashe: get ...` or `gashe: set ...` | as for a failing backend [above](#every-request-goes-to-the-database) |

| `Cache-Status` | Means |
|---|---|
| `gashe; hit` | from the cache; also a stale page [`Stale.WhileRevalidate`](reference/gashe.md#Stale) answered |
| `gashe; fwd=uri-miss; stored` | the handler rendered it for this request, and the cache stored it |
| `gashe; fwd=uri-miss` | the handler rendered it, and it was not stored |
| `gashe; fwd=stale; fwd-status=500` | a stale page, since the handler returned that 5xx |
| `gashe; fwd=method` | past the cache: the method |
| `gashe; fwd=request` | past the cache: `Range`, `Authorization` or `Cookie` |

More: [headers](concepts/httpcache.md#headers),
[how the key is made](concepts/httpcache.md#how-the-key-is-made).

---

## `decode` errors in `OnError` after a deploy { #decode-errors-after-a-deploy }

| Cause | How to tell | Fix |
|---|---|---|
| A field changed its type, such as `int` to `string`: decoding the old entry fails before the shape check | `gashe: decode "<key>": json: cannot unmarshal ...`; the call is a `miss` and stores the new entry | nothing after the rollout; during it, old and new processes overwrite each other: bump [`Policy.Prefix`](reference/gashe.md#Policy), such as `users:v2:` |
| [`Options.Codec`](reference/gashe.md#Options) or [`Options.Compressor`](reference/gashe.md#Options) changed, or differs between processes | decode errors for keys of every type | the same [`Codec`](reference/gashe.md#Codec) and [`Compressor`](reference/gashe.md#Compressor) in every process; the errors stop once every process runs the new one |
| Two call sites read one key as two types | decode errors or misses for that key on every call | a key or [`Policy.Prefix`](reference/gashe.md#Policy) per type |

A renamed or added field decodes fine and is caught by the shape check: a `miss`, with no error.

More: [type shapes](concepts/loading.md#type-shapes),
[change a cached type](recipes/changing-types.md#change-a-cached-type).

---

## The near cache serves an old value after the key expired in Redis { #near-cache-serves-an-expired-value }

| Cause | How to tell | Fix |
|---|---|---|
| A value of the basic API, [`Cache.Set`](reference/gashe.md#Cache.Set) with a TTL shorter than [`LocalTTL`](reference/nearcache.md#Options): the copy lives [`LocalTTL`](reference/nearcache.md#Options) | [`Cache.Get`](reference/gashe.md#Cache.Get) answers after the TTL; `redis-cli` no longer has the key | a [`LocalTTL`](reference/nearcache.md#Options) shorter than the shortest TTL, or [`GetOrLoad`](reference/gashe.md#GetOrLoad), whose entries carry their own expiry |
| Redis evicted the key, or another process deleted it, and there is no `Bus` | the old value for up to [`LocalTTL`](reference/nearcache.md#Options) | `nearcache.Options.Bus` |
| The key was changed or deleted past gashe, and the bus is [`gasheredis.NewBus`](reference/redis.md#NewBus) | as above | [`gasheredis.NewTrackingBus`](reference/redis.md#NewTrackingBus), not on a Cluster |
| The bus lost its connection to Redis | copies stay until it reconnects; then every copy is dropped | none: messages may be lost meanwhile |

More: [backend/nearcache](concepts/backends.md#backendnearcache),
[NewTrackingBus](concepts/backends.md#newtrackingbus).

---

## Keys of the cache collide with yours { #keys-collide }

| Cause | How to tell | Fix |
|---|---|---|
| A key of yours starts with `__`: the cache keeps `__tag:`, `__load:`, `__refresh:` and `__rate:` keys, under [`Options.Prefix`](reference/gashe.md#Options) | tags, locks or rate limits misbehave for those keys | never start a key with `__` |
| [`Cache.Keys`](reference/gashe.md#Cache.Keys) or [`Cache.DeleteMatch`](reference/gashe.md#Cache.DeleteMatch) with `*`: they meet the own keys too, and [`DeleteMatch`](reference/gashe.md#Cache.DeleteMatch) deletes them | `__` keys in what [`Cache.Keys`](reference/gashe.md#Cache.Keys) yields; locks lost, rate counters reset, entries with tags all miss once | patterns of your keys, such as `user:*` |
| A lock key equals a value key: [`Lock`](reference/gashe.md#Cache.Lock), [`TryLock`](reference/gashe.md#Cache.TryLock) and [`WithLock`](reference/gashe.md#Cache.WithLock) use the key you give them | [`TryLock`](reference/gashe.md#Cache.TryLock) is never ok while the value exists; [`Cache.Set`](reference/gashe.md#Cache.Set) over a held lock gives [`ErrLockLost`](reference/gashe.md#variables) | lock keys of their own, such as `lock:reindex` |
| The basic API and [`GetOrLoad`](reference/gashe.md#GetOrLoad) share a key: [`Cache.Get`](reference/gashe.md#Cache.Get) cannot read a [`GetOrLoad`](reference/gashe.md#GetOrLoad) entry | [`GetOrLoad`](reference/gashe.md#GetOrLoad) misses after [`Cache.Set`](reference/gashe.md#Cache.Set), [`Cache.Get`](reference/gashe.md#Cache.Get) reads empty or wrong fields | a [`Policy.Prefix`](reference/gashe.md#Policy) for [`GetOrLoad`](reference/gashe.md#GetOrLoad) data |
| Two services use one Redis with the same keys | each reads what the other stored | an [`Options.Prefix`](reference/gashe.md#Options) per service |

More: [keys](internals/storage.md#keys),
[Prefix](concepts/setup.md#prefix).

---

## Still stuck { #still-stuck }

| You have | Go to |
|---|---|
| an error and want to know what returns it | [Errors](errors.md) |
| a question about how something works | the [guides](concepts/setup.md) and the [API reference](reference.md) |
| an AI assistant to ask about gashe | give it [llms.txt](llms.txt), the index of these pages, or [llms-full.txt](llms-full.txt), every page in one file |
| a bug | [an issue on GitHub](https://github.com/eugeneliukindev/gashe/issues/new) |
| a security problem | [a private report on GitHub](https://github.com/eugeneliukindev/gashe/security/advisories/new), not a public issue |

An issue is fixed fastest when it holds:

- the versions: gashe and its modules (`go list -m all | grep gashe`), Go, Redis or Valkey;
- the backend and the policy you use, as Go code;
- a short program that shows the problem, ideally on the memory backend;
- what you expected, and what happened, with the error or the [`Result`](reference/gashe.md#Result) from [`Options.Hooks.OnResult`](reference/gashe.md#Options).
