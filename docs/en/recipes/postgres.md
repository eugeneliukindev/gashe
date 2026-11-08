# Cache a database query { #cache-a-database-query }

Cache users read from Postgres in Redis, and keep the cache right after an `UPDATE`.

## The code { #the-code }

```bash
go get github.com/eugeneliukindev/gashe github.com/eugeneliukindev/gashe/backend/redis
go get github.com/eugeneliukindev/gashe/codec/msgpack
go get github.com/jackc/pgx/v5 github.com/redis/go-redis/v9
```

The policy, `users.go`:

```go
--8<-- "examples/docs/postgres/users/users.go"
```

1. Postgres is down: answer the stale user for up to an hour.
2. A missing id is asked once per 30 seconds, not on every call.

The read and the write, `load.go`:

```go
--8<-- "examples/docs/postgres/users/load.go"
```

The setup, `main.go`:

```go
--8<-- "examples/docs/postgres/users/main.go:example"
```

1. A slow cache must not slow the service down.
2. Binary: smaller and faster to decode than JSON.

??? example "Full program"

    ```go
    --8<-- "examples/docs/postgres/users/main.go"
    ```

## How it works { #how-it-works }

| Part | What it does |
|---|---|
| `msgpack.Codec{}` | over Redis: smaller values, faster to decode than JSON; no compressor, which makes a small [`User`](../reference/httpcache.md#Vary) bigger and slower; [codec](../concepts/setup.md#codec), [benchmarks](../internals/benchmarks.md#benchmarks) |
| [`GetOrLoad`](../reference/gashe.md#GetOrLoad) | the query runs on a miss only; concurrent calls for one id in a process share it, [`Stampede.LockFor`](../concepts/stampedes.md) makes it one query for all processes |
| `Rename` | the `UPDATE` first, then [`Cached.Set`](../concepts/invalidation.md#cachedset): every process reads the new name next from Redis; a failed `UPDATE` leaves the cache alone |
| [`Stale.IfError`](../reference/gashe.md#Stale) | Postgres is down: the stale user answers for up to an hour; [stale data](../concepts/stale.md#staleiferror-staleifslow) |
| [`Negative`](../reference/gashe.md#Negative) | `sql.ErrNoRows` is cached for 30s; [not found](../concepts/negative.md#negative-cache-not-found) |
| `defer c.Shutdown` | runs before `client.Close`: defers run in reverse order |

## Invalidate with tags { #invalidate-with-tags }

When one write changes many entries, tag them and bump the tag after the write, `orders.go`:

```go
--8<-- "examples/docs/postgres/tags/orders.go"
```

The calls, `main.go`:

```go
--8<-- "examples/docs/postgres/tags/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/postgres/tags/main.go"
    ```

| Part | What it does |
|---|---|
| [`Policy`](../reference/gashe.md#Policy) per call | the tag differs per user, so the policy is built per call and passed to [`gashe.GetOrLoad`](../reference/gashe.md#GetOrLoad) |
| [`InvalidateTags`](../reference/gashe.md#Cache.InvalidateTags) | one `INCRBY` per tag, no scan: every entry with the tag is a miss in every process; [tags](../concepts/invalidation.md#tags) |

## Variations { #variations }

| If you need | Do |
|---|---|
| to invalidate without the new row at hand | `users.Reload(ctx, id, load)` to load and store it now, or `users.Delete(ctx, id)`; [`Reload`](../concepts/invalidation.md#reload) |
| `pgxpool` instead of `database/sql` | call `pool.QueryRow(ctx, ...).Scan(...)` in the loader: gashe sees only the function |
| a page of users by ids | one query for the missing ones: [`GetOrLoadMany`](../concepts/loading.md#getorloadmany) with `WHERE id = ANY($1)` |
| values over about 1 KB, such as pages or long lists | add `Compressor: zstd.New()` from `gashe/compress/zstd`, in a cache of their own; [compressor](../concepts/setup.md#compressor) |
| to keep answering while Redis is down | [`Policy.Fallback`](../concepts/outages.md#fallback) <span class="version-badge">Added in v0.5</span> |
