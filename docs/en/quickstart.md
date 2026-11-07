# Quickstart { #quickstart }

From a first cache to a cache shared by every process of a service, in three steps. Install gashe
first: [Install](index.md#install).

## A first cache { #a-first-cache }

A cache over process memory, and one kind of data with its policy: the loader runs once, then the
cache answers.

```go hl_lines="21 23-26 32"
--8<-- "examples/quickstart/main.go"
```

??? api "API documentation"

    [`gashe.New`](reference/gashe.md#New)<br>
    [`gashe.For`](reference/gashe.md#For)<br>
    [`Cached.GetOrLoad`](reference/gashe.md#Cached[V].GetOrLoad)<br>

## On Redis { #on-redis }

A service runs as many processes, each with its own memory: a memory backend is never shared.
Redis is. Only the backend changes:

```go hl_lines="12-14"
--8<-- "examples/docs/quickstart/redis/main.go"
```

Every process with the same Redis now sees the same entries, tags and locks.

## A policy to start with { #a-policy-to-start-with }

```go hl_lines="18-23"
--8<-- "examples/docs/quickstart/policy/main.go"
```

| Field | What it gives | More |
|---|---|---|
| `TTL` | a result is fresh for an hour | [Loading](concepts/loading.md#ttl) |
| `Stale.WhileRevalidate` | for 10 minutes after that, the stale result answers at once while one process reloads it | [Stale data](concepts/stale.md#stalewhilerevalidate) |
| `Stampede.LockFor` | a cold key loads once for all processes, not once per process | [Stampedes](concepts/stampedes.md#lockonmiss) |
| `Prefix` | users live under `user:`, apart from other kinds of data | [Loading](concepts/loading.md#cacheif-prefix) |

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

    Every type, field and function of each package.

</div>
