# Learn { #learn }

Build a cached user service step by step. Each step adds one feature to the program of the step
before, shows the whole program and what it prints.

| Step | You add | You get |
|---|---|---|
| [1. First cache](first-cache.md) | `gashe.New`, `gashe.For`, `GetOrLoad` | the loader runs once, then the cache answers |
| [2. TTL](ttl.md) | `TTL`, `Jitter` | results that expire |
| [3. Stale data](stale.md) | `StaleWhileRevalidate`, `StaleIfError` | the old result while reloading or while the database fails |
| [4. Invalidation](invalidation.md) | `Tags`, `InvalidateTags`, `Cached.Set` | fresh data right after a write |
| [5. Redis and many pods](redis.md) | `backend/redis`, `LockOnMiss` | one load for all pods |
| [6. Outages](outages.md) | `Options.CircuitBreaker`, `Policy.Fallback` | answers while Redis is down |
| [7. HTTP pages](http.md) | `httpcache` | whole responses cached |

## What you need { #what-you-need }

- Go 1.23 or newer, and a module of your own.
- gashe in it:

```bash
go get github.com/eugeneliukindev/gashe
```

Each program is complete: copy it into `main.go` and run `go run .`. Steps 5 and 6 are about Redis;
their programs simulate it in memory, so every step runs without a server.
