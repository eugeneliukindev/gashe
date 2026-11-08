# Settings { #settings }

Every field you can set, nested ones included, as you write them in code: each line says what
the field does and what leaving it out means. Every field may be left out unless it says
required.

## Options { #options }

The options of a cache, given to [`gashe.New`](reference/gashe.md#New).

```go
--8<-- "examples/docs/settings/options/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/options/main.go"
    ```

| Field | Cases it covers |
| --- | --- |
| `Prefix` | the orders and the billing services cache rows of their own databases in one backend |
| `Codec` | rows with large `JSONB` columns: MessagePack is smaller and faster to decode than JSON |
| `Compressor` | a row with a long text column, such as a product description or a report of 2 MB |
| `CircuitBreaker` | the cache backend does not answer: no call waits for its timeout before it queries the database |
| `LockPollInterval` | a load under `LockFor` takes 50ms: the processes that wait need not check every 100ms |
| `Hooks` | the hit ratio and the time of `SELECT`s in Prometheus; a log line when a down replica is answered with old rows |

Reference: [`Options`](reference/gashe.md#Options), [`CircuitBreaker`](reference/gashe.md#CircuitBreaker), [`Hooks`](reference/gashe.md#Hooks).

## Policy { #policy }

The rules of one kind of data, given to [`gashe.For`](reference/gashe.md#For),
[`GetOrLoad`](reference/gashe.md#GetOrLoad) and [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany).

```go
--8<-- "examples/docs/settings/policy/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/policy/main.go"
    ```

| Field | Cases it covers |
| --- | --- |
| `TTL.For` | a profile changes once a day: an hour; stock changes often: 10 seconds; a row with its own `expires_at`: a func |
| `TTL.Jitter` | 10 000 products warmed up by one query would expire in one second; a nightly job loaded the whole catalog |
| `Prefix` | `user:42` and `order:42` are rows of two tables with the same id; `users:v2:` after a migration changed the columns |
| `Stale.WhileRevalidate` | a top-products query with `JOIN` and `ORDER BY` takes 300ms: the last list answers at once; dashboard aggregates |
| `Stale.IfError` | the replica is down for 5 minutes; the connection pool is exhausted; a query times out under load |
| `Stale.IfError.IsFailure` | `sql.ErrNoRows` is an answer, not a failure: a deleted row must not show up old; so is `permission denied` |
| `Stale.IfSlow` | a query waits for a table lock or a `VACUUM`; a heavy report under load |
| `Stampede.SingleFlight` | 200 requests for one product page at once in a process make one `SELECT` |
| `Stampede.LockFor` | 20 pods restarted together read a cold key: one `SELECT` for all of them; a costly report computed once |
| `Stampede.MaxLoads` | a pool of 20 connections and 5000 cold keys after a flush: at most 20 queries run at once |
| `Tags` | an `UPDATE` of an order drops the profile, the order list and the cart of its user; a migration drops every user |
| `Negative` | bots try ids that do not exist; links to a deleted product: without it, each one is a `SELECT` that finds no row |
| `Fallback` | the cache backend is down: without it every call queries the database, with it one query per key and process |
| `Store.If` | an empty order list is not kept, an order appears a second later; a row in status `draft` is not kept |
| `Store.SlowerThan` | a lookup by primary key in 2ms is not cached, a full-text search in 800ms is |

Reference: [`Policy`](reference/gashe.md#Policy), [`TTL`](reference/gashe.md#TTL), [`Stale`](reference/gashe.md#Stale), [`Stampede`](reference/gashe.md#Stampede), [`Negative`](reference/gashe.md#Negative), [`Fallback`](reference/gashe.md#Fallback), [`Store`](reference/gashe.md#Store).

## HTTP caching { #httpcache }

The options of the middleware, given to [`httpcache.New`](reference/httpcache.md#New).

```go
--8<-- "examples/docs/settings/httpcache/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/httpcache/main.go"
    ```

| Field | Cases it covers |
| --- | --- |
| `Policy` | a product page built from 6 queries |
| `Policy.Store.If` | the 404 of a deleted product is kept, the 500 of a database that is down is not |
| `Vary.Query` | `?page=2` is another `OFFSET`; `?utm_source=` changes no query |
| `Vary.Headers` | `Accept-Language` picks the column of the translation |
| `Vary.Cookies` | the currency in a cookie changes the prices a query returns |
| `Vary.User` | a "my orders" page: `WHERE user_id = ...` |
| `MaxBodyBytes` | an export of the whole catalog, 50 MB, stays out of the cache |

Reference: [`httpcache.Options`](reference/httpcache.md#Options), [`Vary`](reference/httpcache.md#Vary).

## Backends { #backends }

The options of the memory backend and of the near cache.

```go
--8<-- "examples/docs/settings/backends/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/backends/main.go"
    ```

| Field | Cases it covers |
| --- | --- |
| `memory.Options.MaxEntries`, `MaxBytes` | rows of a catalog in the memory of a small container |
| `memory.Options.PurgeInterval` | many rows with a short TTL, such as stock, purged more often |
| `nearcache.Options.LocalTTL` | a hot row, such as the settings of a shop, read on every request |
| `nearcache.Options.Bus` | after an `UPDATE`, every process drops its copy of the row at once |

Reference: [`memory.Options`](reference/memory.md#Options), [`nearcache.Options`](reference/nearcache.md#Options). The Redis backend takes only its client: [`redis.New`](reference/redis.md#New).
