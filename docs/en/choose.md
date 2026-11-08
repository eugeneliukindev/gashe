# Which setting { #which-setting }

Find the problem, then the setting that solves it and the section that shows it. Every field at
once is in [Settings](settings.md).

## Load and store { #load-and-store }

| Problem | For example | Setting | Where |
| --- | --- | --- | --- |
| The same expensive call runs again and again | a user profile read from Postgres on every request | `gashe.For`, `GetOrLoad`, `Policy.TTL.For` | [Read, or load and store](concepts/loading.md#getorload) |
| A page loads many keys, one query each | a catalog page with 50 products, 50 queries | `GetOrLoadMany` | [Many keys in one query](concepts/loading.md#getorloadmany) |
| Empty results or some values must not be stored | an empty order list, filled a second later | `Policy.Store.If` | [What is stored](concepts/loading.md#cacheif-prefix) |
| A source answers fast, and only its slow answers are worth caching | an index lookup of 2ms next to a full-text search of 800ms | `Policy.Store.SlowerThan` | [What is stored](concepts/loading.md#cacheif-prefix) |
| Lookups of rows that do not exist reach the database every time | bots that try user ids that do not exist | `Policy.Negative` | [Caching "not found"](concepts/negative.md#negative-cache-not-found) |

## Many calls at once { #many-calls-at-once }

| Problem | For example | Setting | Where |
| --- | --- | --- | --- |
| A cold key sends one query per concurrent call of a process | 200 requests for one product page at once | `Policy.Stampede.SingleFlight` | [One load per process](concepts/stampedes.md#singleflight) |
| Every process loads the same cold key at once | 20 pods restarted together read the same cold key | `Policy.Stampede.LockFor` | [One load for all processes](concepts/stampedes.md#lockonmiss) |
| A thousand cold keys load at once after a deploy | 5000 reports after a flush run out the connection pool | `Policy.Stampede.MaxLoads` | [A limit on loads at once](concepts/stampedes.md#maxconcurrentloads) |
| Entries stored together expire together | 10 000 products warmed up at once expire in one second | `Policy.TTL.Jitter` | [Spread the expiry](concepts/stampedes.md#jitter) |

## Slow or failing sources { #slow-or-failing-sources }

| Problem | For example | Setting | Where |
| --- | --- | --- | --- |
| A call waits for the reload once the TTL is over | a top-products query of 300ms on the home page | `Policy.Stale.WhileRevalidate` | [Refresh in the background](concepts/stale.md#stalewhilerevalidate) |
| The source fails and calls get its error | the Postgres replica is down for 5 minutes | `Policy.Stale.IfError` | [Answer instead of an error](concepts/stale.md#staleiferror-staleifslow) |
| The source hangs and every call waits for it | a report query stuck behind a table lock for 10 seconds | `Policy.Stale.IfSlow` | [Answer instead of a slow loader](concepts/stale.md#stale-ifslow) |
| The backend is down and every call waits for its timeout | the cache backend stopped, each call waits before its query | `Options.CircuitBreaker` | [Stop calling a backend that is down](concepts/outages.md#circuitbreaker) |
| The backend is down and the database gets every call | the cache backend stopped, and every request queries Postgres | `Policy.Fallback` | [Answer from the memory of the process](concepts/outages.md#fallback) |
| A dependency fails and you keep calling it | a reporting database that times out keeps getting queries | `gashe.Guard` with a `CircuitBreaker` | [Guarding a call](concepts/breakers.md#gasheguard) |

## Changes to the data { #changes-to-the-data }

| Problem | For example | Setting | Where |
| --- | --- | --- | --- |
| A change must remove every result it touches, across kinds of data | an order changed: the profile, the order list and the cart of its user | `Policy.Tags`, `InvalidateTags` | [Update and remove data](concepts/invalidation.md#tags) |
| You wrote the new value and the cache should have it at once | an UPDATE of a profile in the same handler | `Cached.Set` | [Writing a new result](concepts/invalidation.md#cachedset) |
| The cache should load the new value now | a price another service updated, reloaded on its event | `Cached.Reload` | [Reloading after a change](concepts/invalidation.md#reload) |
| A result must go | a deleted account | `Cached.Delete` | [Deleting a result](concepts/invalidation.md#cacheddelete) |

## The cache itself { #the-cache-itself }

| Problem | For example | Setting | Where |
| --- | --- | --- | --- |
| Several services share one backend | the orders and the billing databases both have a row 42 | `Options.Prefix` | [Key prefix](concepts/setup.md#prefix) |
| Values are stored in another format than JSON | rows with large `JSONB` columns, smaller in MessagePack | `Options.Codec` | [Value format](concepts/setup.md#codec) |
| Values are large | a row with a long text column, such as a report of 2 MB | `Options.Compressor` | [Compression](concepts/setup.md#compressor) |
| You need metrics and logs of hits, loads and errors | the hit ratio and the time of `SELECT`s in Prometheus | `Options.Hooks` | [Metrics and logs](concepts/setup.md#onresult-onerror) |
| Hot keys cost a round trip to a shared backend | the settings row of a shop read on every request | `nearcache.New` | [A near cache in front of a shared backend](concepts/backends.md#backendnearcache) |
| Large values take too much memory of the process | big catalog pages in a small container | `memory.Options.MaxBytes` | [Memory of the process](concepts/backends.md#backendmemory) |

## Beyond caching { #beyond-caching }

| Problem | For example | Setting | Where |
| --- | --- | --- | --- |
| Only one process may run a job at a time | a nightly reindex on 20 pods | `WithLock`, `TryLock` | [Run work under a lock](concepts/locks.md#cwithlock) |
| A client makes too many calls | a search endpoint that runs a heavy query per request | `RateLimit` | [Limit calls per key](concepts/rate-limits.md#cratelimit) |
| The pages of an HTTP handler are expensive to render | a product page built from 6 queries | `httpcache.New` | [Caching the pages of a handler](concepts/httpcache.md#httpcachenew) |
