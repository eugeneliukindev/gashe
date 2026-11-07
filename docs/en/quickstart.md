# Quickstart { #quickstart }

From a first cache to a cache shared by every process of a service, in three steps. Install gashe
first: [Install](index.md#install).

## A first cache { #a-first-cache }

A cache over process memory, and one kind of data with its policy: the loader runs once, then the
cache answers. [`gashe.New`](reference/gashe.md#New) creates the cache,
[`gashe.For`](reference/gashe.md#For) binds the policy to it, and
[`GetOrLoad`](reference/gashe.md#Cached[V].GetOrLoad) reads or loads.

```go hl_lines="21 23-26 32"
--8<-- "examples/quickstart/main.go"
```

## On Redis { #on-redis }

A service runs as many processes, each with its own memory: a memory backend is never shared.
Redis is. Only the backend changes:

```go hl_lines="12-14"
--8<-- "examples/docs/quickstart/redis/main.go"
```

Every process with the same Redis now sees the same entries, tags and locks.

## A policy to start with { #a-policy-to-start-with }

A [`gashe.Policy`](concepts/loading.md#policy) holds the rules of one kind of data. These are the
fields most services set first; every field is in [The policy](concepts/loading.md#policy).

```go hl_lines="18-23"
--8<-- "examples/docs/quickstart/policy/main.go"
```

1. A result is fresh for an hour.
2. Users live under `user:`, apart from other kinds of data.
3. For 10 minutes after that, the stale result answers at once while one process reloads it.
4. A cold key loads once for all processes, not once per process.
