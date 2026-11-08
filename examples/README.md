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
| [stampede](stampede/main.go) | a thousand concurrent requests, one load |
| [batch](batch/main.go) | `GetOrLoadMany`: one query for the missing items of a list |
| [tags](tags/main.go) | `InvalidateTags` after a write |
| [nearcache](nearcache/main.go) | two caches standing in for two processes, each with a near cache and a `Bus`; needs Redis |
| [httpserver](httpserver/main.go) | `httpcache`: `Cache-Status`, `ETag`, `304` |
| [httpapi](httpapi) | `httpcache` behind real servers, for curl: `Vary` and cookies in [products](httpapi/products/main.go), `Stale.IfError` in [report](httpapi/report/main.go), `503` in [export](httpapi/export/main.go), `RateLimit` in [login](httpapi/login/main.go) |
| [lock](lock/main.go) | `c.TryLock`: one worker runs a job |
| [ratelimit](ratelimit/main.go) | fixed window with a ban, sliding window |
| [breaker](breaker/main.go) | `CircuitBreaker` opening and closing |
| [monitoring](monitoring/main.go) | `OnResult`: hit ratio and loader time |
| [quickstart](quickstart/main.go) | the program of the [quickstart](../docs/en/quickstart.md): `GetOrLoad`, the loader once, then hits |

Every example uses the memory backend except `nearcache`, which needs a Redis on
`localhost:6379`, such as `docker run --rm -p 6379:6379 redis:7`.

The examples are a separate module, so their dependencies never reach the gashe modules; its
`go.mod` replaces gashe with the code of this repository.
