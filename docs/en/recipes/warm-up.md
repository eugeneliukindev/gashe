# Warm up the cache { #warm-up-the-cache }

Fill an empty cache, after a deploy or in a new region, before the traffic does it, without flooding
the database.

<span class="version-badge">Added in v0.5</span> `Stampede.MaxLoads` and `ErrOverloaded`.

## The code { #the-code }

Files of one `package main`. The policy and the warm-up:

```go hl_lines="16 30"
--8<-- "examples/docs/warm-up/warmup.go"
```

1. `Stampede.MaxLoads` holds the rest back.

A refresh, for a job that renews hot entries before they expire:

```go hl_lines="21"
--8<-- "examples/docs/warm-up/refresh.go"
```

`main` runs both against a stand-in for the database:

```go
--8<-- "examples/docs/warm-up/main.go"
```

1. In production: `SELECT id FROM products ORDER BY views DESC LIMIT 2000`.

## How it works { #how-it-works }

| Part | What it does |
|---|---|
| `GetOrLoadMany` per 100 ids | one query per batch, for the ids the cache does not have: a second run makes no query; [`GetOrLoadMany`](../concepts/loading.md#getorloadmany) |
| `Stampede.MaxLoads: 4` | at most 4 queries at once in this process, whatever the number of goroutines; a batch takes one slot; [load limit](../concepts/stampedes.md#maxconcurrentloads) |
| `context.WithTimeout` | a batch that waits for a slot past the deadline gets `ErrOverloaded` instead of waiting forever |
| `Jitter: 0.2` | entries warmed together expire from 48m to 72m, not at once; [jitter](../concepts/stampedes.md#jitter) |
| `Reload` in `refresh` | loads and stores every id, whatever is stored: a job that renews hot entries before they expire; [`Reload`](../concepts/invalidation.md#reload) |

## Variations { #variations }

| If you need | Do |
|---|---|
| a new region | point the warm-up at the cache of the region before it gets traffic; take the hot ids from the database or the logs of a live region |
| one warm-up for all processes | run it in one job, or in every process under [`Cache.TryLock`](../concepts/locks.md#ctrylock) |
| to warm a policy with `Stale`, `Stampede.LockFor` or `Negative` | `GetOrLoadMany` returns `ErrInvalidOptions` for them: warm with a copy without them, as below (a second file of the same `package main`: it uses `Product`); `GetOrLoad` with the full policy reads what it stored |

```go
--8<-- "examples/docs/warm-up/stale.go"
```

1. `GetOrLoadMany` refuses stale windows, `Stampede.LockFor` and `Negative`: a copy without them.

The warmed entries live for `TTL` only, without the stale window, until the next load stores them
with the full policy.
