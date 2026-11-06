# Quickstart { #quickstart }

From the install to a cache shared by every process of a service, in four steps.

## Install { #install }

```bash
go get github.com/eugeneliukindev/gashe
go get github.com/eugeneliukindev/gashe/backend/redis # the Redis backend
```

Go 1.23 or newer. Some features need v0.5, not released yet: they are marked
<span class="version-badge">Added in v0.5</span>.

## A first cache { #a-first-cache }

A cache over process memory, and one kind of data with its policy: the loader runs once, then the
cache answers.

```go hl_lines="22 27 28 29 30 39"
--8<-- "examples/quickstart/main.go"
```

```text
--8<-- "docs/snippets/examples/quickstart.txt"
```

| Line | What it does |
|---|---|
| `gashe.New(mem, ...)` | a cache over a backend, here the memory of this process |
| `gashe.For(c, gashe.Policy[User]{...})` | binds a policy to the cache: how long a user is fresh, under which prefix |
| `users.GetOrLoad(ctx, "42", loadUser)` | answers from the cache, or calls the loader and stores its result |

## On Redis { #on-redis }

A service runs as many processes, each with its own memory: a memory backend is never shared.
Redis is. Only the backend changes:

```go
client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
defer client.Close()
c := gashe.New(gasheredis.New(client), gashe.Options{})
```

Every process with the same Redis now sees the same entries, tags and locks.

## A policy to start with { #a-policy-to-start-with }

```go
users := gashe.For(c, gashe.Policy[User]{
	TTL:                  time.Hour,
	StaleWhileRevalidate: 10 * time.Minute,
	LockOnMiss:           5 * time.Second,
	Prefix:               "user:",
})
```

| Field | What it gives | More |
|---|---|---|
| `TTL` | a result is fresh for an hour | [Loading](concepts/loading.md#ttl-how-long-a-result-is-fresh) |
| `StaleWhileRevalidate` | for 10 minutes after that, the stale result answers at once while one process reloads it | [Stale data](concepts/stale.md#stalewhilerevalidate-answer-old-data-while-reloading) |
| `LockOnMiss` | a cold key loads once for all processes, not once per process | [Stampedes](concepts/stampedes.md#lockonmiss-one-load-for-all-processes) |
| `Prefix` | users live under `user:`, apart from other kinds of data | [Loading](concepts/loading.md#cacheif-prefix-what-to-store-and-under-which-key) |

## Next { #next }

<div class="grid cards" markdown>

-   **[Concepts](concepts/setup.md)**

    ---

    One page per problem: stale data, stampedes, outages, invalidation, locks, limits.

-   **[Recipes](recipes/index.md)**

    ---

    Ready code for Postgres, external APIs, metrics, tests and warm-up.

-   **[Troubleshooting](troubleshooting.md)**

    ---

    A symptom, its cause and the fix.

-   **[API reference](reference.md)**

    ---

    Every package on pkg.go.dev.

</div>
