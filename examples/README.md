# Examples

Runnable programs, one per feature. The examples are a module of their own, so run them from
`examples/`:

```bash
cd examples && go run ./stale/while-revalidate
```

| Example | Shows |
|---|---|
| [basic](basic/main.go) | `Set`, `Get`, `GetMany`, `IncrBy`, `TTL` |
| [cacheaside](cacheaside/main.go) | `GetOrLoad`: one query, then answers from the cache |
| [stale](stale) | one key past its TTL: [ttl-only](stale/ttl-only/main.go), [while-revalidate](stale/while-revalidate/main.go), [if-error](stale/if-error/main.go), [if-slow](stale/if-slow/main.go) |
| [staleif](staleif) | `Stale.IfError.IsFailure`: which errors the stale result answers, [without](staleif/without/main.go) and [with](staleif/with/main.go) |
| [negative](negative) | caching "not found": [without](negative/without/main.go) and [with](negative/with/main.go) `Negative` |
| [stampede](stampede/main.go) | a thousand concurrent requests, one load |
| [batch](batch/main.go) | `GetOrLoadMany`: one query for the missing items of a list |
| [tags](tags/main.go) | `InvalidateTags` after a write |
| [nearcache](nearcache/main.go) | two caches standing in for two processes, each with a near cache and a `Bus`; needs Redis |
| [httpserver](httpserver/main.go) | `httpcache`: `Cache-Status`, `ETag`, `304` |
| [httpapi](httpapi/main.go) | `httpcache` behind a real server, for curl: `Vary`, cookies, `Stale.IfError`, `503`; needs Redis |
| [lock](lock/main.go) | `c.TryLock`: one worker runs a job |
| [ratelimit](ratelimit/main.go) | fixed window with a ban, sliding window |
| [breaker](breaker/main.go) | `CircuitBreaker` opening and closing |
| [fallback](fallback) | `Policy.Fallback`: one database query instead of a hundred while Redis is down, [without](fallback/without/main.go) and [with](fallback/with/main.go) |
| [monitoring](monitoring/main.go) | `OnResult`: hit ratio and loader time |
| [quickstart](quickstart/main.go) | the program of the [quickstart](../docs/en/quickstart.md): `GetOrLoad`, the loader once, then hits |
| [lockfor](lockfor) | three processes over one Redis, [without](lockonmiss/without/main.go) and [with](lockonmiss/with/main.go) `Stampede.LockFor`, which loads a cold key once for all; needs Redis |

Every example uses the memory backend except `nearcache` and `lockonmiss`, which need a Redis on
`localhost:6379`, such as `docker run --rm -p 6379:6379 redis:7`.

The examples are a separate module, so their dependencies never reach the gashe modules; its
`go.mod` replaces gashe with the code of this repository.
