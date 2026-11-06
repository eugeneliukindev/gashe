# gashe { #gashe }

<p align="center">
  <img src="assets/logo.svg" width="180" alt="The gashe gopher, its cheeks full of cached data">
</p>

Caching for Go services that run as many processes. One `*gashe.Cache` stores the results of
expensive calls, keeps answering while a dependency is slow or down, and guards dependencies with
locks, rate limits and circuit breakers. Over Redis, all of it is shared by every pod.

## Why gashe { #why-gashe }

- **One load, not a thousand:** concurrent calls for a key share one load, and with `LockOnMiss`
  one pod loads for all of them.
- **Answers while things fail:** the last result while the database is slow or down, memory while
  Redis is.
- **Invalidation that works:** tags drop many entries with one write; a near cache sees changes
  made in Redis by anyone.
- **Guards for dependencies:** distributed locks, rate limits and circuit breakers on the same cache.
- **HTTP responses:** a middleware that caches whole pages, with `ETag`, `304` and `Cache-Status`.
- **No dependencies:** the core module needs nothing but the standard library, Go 1.23 or newer.

## Install { #install }

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/backend/redis # Redis backend, on go-redis v9
```

## Example { #example }

A user is loaded once, then answered from Redis by every pod for an hour; after that, the old
result answers at once while one pod reloads it.

```go
package main

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

type User struct {
	ID   int
	Name string
}

func main() {
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	c := gashe.New(gasheredis.New(client), gashe.Options{Prefix: "app:"})
	defer c.Shutdown(ctx)

	users := gashe.For(c, gashe.Policy[User]{
		TTL:                  time.Hour,
		StaleWhileRevalidate: 10 * time.Minute, // answer at once, reload in the background
		LockOnMiss:           5 * time.Second,  // a cold key loads once for all pods
		Prefix:               "user:",
	})

	u, err := users.GetOrLoad(ctx, "42", func(ctx context.Context) (User, error) {
		return User{ID: 42, Name: "Ann"}, nil // your database query
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(u.Name)
}
```

## Next { #next }

<div class="grid cards" markdown>

-   **[Learn](tutorial/index.md)**

    ---

    Eight short steps, from a first cache to Redis outages and HTTP pages.

-   **[Recipes](recipes/index.md)**

    ---

    Ready code for Postgres, external APIs, metrics, tests and warm-up.

-   **[Concepts](concepts/setup.md)**

    ---

    One page per problem: stale data, stampedes, outages, invalidation, locks, limits.

-   **[Errors](errors.md)**

    ---

    Every error gashe returns, and when.

-   **[API reference](reference.md)**

    ---

    Every package on pkg.go.dev.

-   **[How it stores data](internals/storage.md)**

    ---

    Keys, entries and what each strategy keeps in Redis.

</div>

## What it solves { #what-it-solves }

| Problem | Known as | Solution |
|---|---|---|
| The same expensive call again and again | [Memoization](https://en.wikipedia.org/wiki/Memoization) | [`GetOrLoad`](concepts/loading.md#getorload) |
| Every request waits when a popular key expires | [Cache stampede](https://en.wikipedia.org/wiki/Cache_stampede) on a hot key | [`StaleWhileRevalidate`](concepts/stale.md#stalewhilerevalidate-answer-old-data-while-reloading) |
| The database is down and the cache is empty | [Graceful degradation](https://en.wikipedia.org/wiki/Graceful_degradation) | [`StaleIfError`](concepts/stale.md#staleiferror-staleifslow-answer-old-data-while-the-source-fails) |
| The database is slow, and every request waits for it | [Graceful degradation](https://en.wikipedia.org/wiki/Graceful_degradation) | [`StaleIfSlow`](concepts/stale.md#staleiferror-staleifslow-answer-old-data-while-the-source-fails) |
| Lookups of missing ids always reach the database | [Negative cache](https://en.wikipedia.org/wiki/Negative_cache), cache penetration | [`Negative`](concepts/negative.md#negative-cache-not-found) |
| A heavy page is rendered on every request | [Web cache](https://en.wikipedia.org/wiki/Web_cache) | [`httpcache`](concepts/httpcache.md#httpcache-whole-http-responses) |
| One change makes many entries stale | [Cache invalidation](https://en.wikipedia.org/wiki/Cache_invalidation) | [`Tags`, `InvalidateTags`](concepts/invalidation.md#tags-invalidate-many-entries-with-one-write) |
| A thousand different cold keys flood the database at once | [Bulkhead pattern](https://en.wikipedia.org/wiki/Bulkhead_pattern), load shedding | [`MaxConcurrentLoads`](concepts/stampedes.md#maxconcurrentloads-protect-the-source-from-too-many-loads-at-once) |
| A cold key: a thousand identical queries | [Cache stampede](https://en.wikipedia.org/wiki/Cache_stampede), dog-piling | built in for one process, [`LockOnMiss`](concepts/stampedes.md#lockonmiss-one-load-for-all-pods) for all pods |
| N+1 queries on a list page | N+1 query problem | [`GetOrLoadMany`](concepts/lists.md#getorloadmany) |
| Keys stored together expire together | [Thundering herd](https://en.wikipedia.org/wiki/Thundering_herd_problem), cache avalanche | [`Jitter`](concepts/stampedes.md#jitter-spread-expiry) |
| A hot key costs a round trip on every read | Hot key | [`nearcache`](concepts/backends.md#backendnearcache) |
| Work must run once at a time across pods | [Mutual exclusion](https://en.wikipedia.org/wiki/Mutual_exclusion), [distributed lock](https://en.wikipedia.org/wiki/Distributed_lock_manager) | [`WithLock`, `TryLock`](concepts/locks.md#locks) |
| A client calls far too often | [Rate limiting](https://en.wikipedia.org/wiki/Rate_limiting) | [`RateLimit`](concepts/rate-limits.md#cratelimit) |
| A failing dependency drags the service down | [Cascading failure](https://en.wikipedia.org/wiki/Cascading_failure) | [`Guard`, `CircuitBreaker`](concepts/breakers.md#gasheguard) |
| The cache backend is down, and every request goes to the database | [Graceful degradation](https://en.wikipedia.org/wiki/Graceful_degradation), gutter pool | [`Policy.Fallback`](concepts/outages.md#fallback-answer-from-memory-while-the-backend-is-down) |
| The cache backend is down, and every request waits for its timeout | [Cascading failure](https://en.wikipedia.org/wiki/Cascading_failure) | [`Options.CircuitBreaker`](concepts/outages.md#circuitbreaker-keep-working-when-the-backend-is-down) |
| A deploy changed a cached type | [Schema evolution](https://en.wikipedia.org/wiki/Schema_evolution) | built in: [shapes](concepts/loading.md#type-shapes-type-changes-after-a-deploy) |
