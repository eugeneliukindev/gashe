# Examples

Runnable programs, one per feature. Run one from the repository root:

```bash
go run ./examples/stale
```

| Example | Shows |
|---|---|
| [basic](basic/main.go) | `Set`, `Get`, `GetMany`, `IncrBy`, `TTL` |
| [cacheaside](cacheaside/main.go) | `GetOrLoad`: one query, then answers from the cache |
| [stale](stale/main.go) | stale-while-revalidate, stale-if-error and stale-if-slow |
| [negative](negative/main.go) | caching "not found" |
| [stampede](stampede/main.go) | a thousand concurrent requests, one load |
| [batch](batch/main.go) | `GetOrLoadMany`: one query for the missing items of a list |
| [tags](tags/main.go) | `InvalidateTags` after a write |
| [nearcache](nearcache/main.go) | two pods, local caches and a `Bus`; needs Redis |
| [httpserver](httpserver/main.go) | `httpcache`: `Cache-Status`, `ETag`, `304` |
| [lock](lock/main.go) | `TryLock`: one worker runs a job |
| [ratelimit](ratelimit/main.go) | fixed window with a ban, sliding window |
| [breaker](breaker/main.go) | `CircuitBreaker` opening and closing |
| [fallback](fallback/main.go) | `Policy.Fallback`: one database query instead of a hundred while Redis is down |
| [monitoring](monitoring/main.go) | `OnResult`: hit ratio and loader time |

Every example uses the memory backend except `nearcache`, which needs Redis:
`docker run --rm -p 6379:6379 redis:7`, or set `REDIS_ADDR`.

The examples are a separate module, so their dependencies never reach the gashe modules; its
`go.mod` replaces gashe with the code of this repository.
