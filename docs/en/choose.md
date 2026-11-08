# Which setting { #which-setting }

Find the problem, then the setting that solves it and the section that shows it.

## Load and store { #load-and-store }

| Problem | Setting | Where |
| --- | --- | --- |
| The same expensive call runs again and again | `gashe.For`, `GetOrLoad`, `Policy.TTL.For` | [Read, or load and store](concepts/loading.md#getorload) |
| A page loads many keys, one query each | `GetOrLoadMany` | [Many keys in one query](concepts/loading.md#getorloadmany) |
| Empty results or some values must not be stored | `Policy.Store.If` | [What is stored](concepts/loading.md#cacheif-prefix) |
| A source answers fast, and only its slow answers are worth caching | `Policy.Store.SlowerThan` | [What is stored](concepts/loading.md#cacheif-prefix) |
| Lookups of rows that do not exist reach the database every time | `Policy.Negative` | [Caching "not found"](concepts/negative.md#negative-cache-not-found) |

## Many calls at once { #many-calls-at-once }

| Problem | Setting | Where |
| --- | --- | --- |
| A cold key sends one query per concurrent call of a process | `Policy.Stampede.SingleFlight` | [One load per process](concepts/stampedes.md#singleflight) |
| Every process loads the same cold key at once | `Policy.Stampede.LockFor` | [One load for all processes](concepts/stampedes.md#lockonmiss) |
| A thousand cold keys load at once after a deploy | `Policy.Stampede.MaxLoads` | [A limit on loads at once](concepts/stampedes.md#maxconcurrentloads) |
| Entries stored together expire together | `Policy.TTL.Jitter` | [Spread the expiry](concepts/stampedes.md#jitter) |

## Slow or failing sources { #slow-or-failing-sources }

| Problem | Setting | Where |
| --- | --- | --- |
| A call waits for the reload once the TTL is over | `Policy.Stale.WhileRevalidate` | [Refresh in the background](concepts/stale.md#stalewhilerevalidate) |
| The source fails and calls get its error | `Policy.Stale.IfError` | [Answer instead of an error](concepts/stale.md#staleiferror-staleifslow) |
| The source hangs and every call waits for it | `Policy.Stale.IfSlow` | [Answer instead of a slow loader](concepts/stale.md#stale-ifslow) |
| Redis is down and every call waits for its timeout | `Options.CircuitBreaker` | [Stop calling a backend that is down](concepts/outages.md#circuitbreaker) |
| Redis is down and the database gets every call | `Policy.Fallback` | [Answer from the memory of the process](concepts/outages.md#fallback) |
| A dependency fails and you keep calling it | `gashe.Guard` with a `CircuitBreaker` | [Guarding a call](concepts/breakers.md#gasheguard) |

## Changes to the data { #changes-to-the-data }

| Problem | Setting | Where |
| --- | --- | --- |
| A change must remove every result it touches, across kinds of data | `Policy.Tags`, `InvalidateTags` | [Update and remove data](concepts/invalidation.md#tags) |
| You wrote the new value and the cache should have it at once | `Cached.Set` | [Writing a new result](concepts/invalidation.md#cachedset) |
| The cache should load the new value now | `Cached.Reload` | [Reloading after a change](concepts/invalidation.md#reload) |
| A result must go | `Cached.Delete` | [Deleting a result](concepts/invalidation.md#cacheddelete) |

## The cache itself { #the-cache-itself }

| Problem | Setting | Where |
| --- | --- | --- |
| Several services share one Redis | `Options.Prefix` | [Key prefix](concepts/setup.md#prefix) |
| Values are stored in another format than JSON | `Options.Codec` | [Value format](concepts/setup.md#codec) |
| Values are large | `Options.Compressor` | [Compression](concepts/setup.md#compressor) |
| You need metrics and logs of hits, loads and errors | `Options.Hooks` | [Metrics and logs](concepts/setup.md#onresult-onerror) |
| Hot keys cost a round trip to Redis | `nearcache.New` | [A near cache in front of Redis](concepts/backends.md#backendnearcache) |
| Large values take too much memory of the process | `memory.Options.MaxBytes` | [Memory of the process](concepts/backends.md#backendmemory) |

## Beyond caching { #beyond-caching }

| Problem | Setting | Where |
| --- | --- | --- |
| Only one process may run a job at a time | `WithLock`, `TryLock` | [Run work under a lock](concepts/locks.md#cwithlock) |
| A client makes too many calls | `RateLimit` | [Limit calls per key](concepts/rate-limits.md#cratelimit) |
| The pages of an HTTP handler are expensive to render | `httpcache.New` | [Caching the pages of a handler](concepts/httpcache.md#httpcachenew) |
