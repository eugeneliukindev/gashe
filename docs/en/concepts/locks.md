# Locks { #locks }

Let one caller across all processes run work that must never run twice at once, such as a daily
report or a payment.

## `c.WithLock`: run a function under a lock { #cwithlock }

Runs a function while holding the lock, and keeps the lock as long as the function runs.

API: [`gashe.Cache.WithLock`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.WithLock)

```go
err := c.WithLock(ctx, "job:reindex", time.Minute, func(ctx context.Context) error {
	return reindex(ctx)
})
```

```mermaid
sequenceDiagram
    participant W as WithLock
    participant R as Redis
    participant F as reindex
    W->>R: 0s: SET job:reindex id NX EX 60
    W->>F: reindex(ctx) starts
    W->>R: 20s: PEXPIRE 1m if the id is ours
    W->>R: 40s: PEXPIRE 1m if the id is ours
    F-->>W: 55s: reindex returns
    W->>R: DEL if the id is ours
```

| What | How |
|---|---|
| The lock | a key in the backend that holds the random id of its holder, shared by every process; its TTL frees it if the holder dies |
| Default | the only option is the TTL; `Options.LockPollInterval`, 100ms, sets how often a busy lock is tried again |
| How | takes the lock as `c.Lock` does, runs the function with a `ctx` of its own, extends the lock every third of the TTL and releases it when the function returns |
| Returns | the error of the function |
| Errors | `ErrLockLost` - an extension found the lock gone, its `ctx` was cancelled, and the function returned nil or an error that wraps `context.Cause(ctx)`; a function that returns `ctx.Err()` gives `context.Canceled`. A backend error of an extension follows the same rule and is not turned into `ErrLockLost`. `ctx.Err()` - the wait ended first. `ErrInvalidTTL` - the TTL is not positive. Backend errors while taking the lock |
| Unlock errors | go to `OnError` |
| Redis | as `c.Lock`, `lease.Extend` and `lease.Unlock` |

!!! warning
    Locks are advisory: a holder paused longer than the TTL (GC, a frozen VM) can overlap with the next
    one. Make writes that guard money idempotent too.

- **Example:** [examples/lock](https://github.com/eugeneliukindev/gashe/blob/main/examples/lock/main.go)

???+ example "examples/lock/main.go"

    ```go hl_lines="21 25 30-31"
    --8<-- "examples/lock/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/lock.txt"
    ```

---

## `c.TryLock`: take a free lock or skip the work { #ctrylock }

Takes the lock if it is free, without waiting.

API: [`gashe.Cache.TryLock`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.TryLock)

```go
lease, ok, err := c.TryLock(ctx, "job:reindex", time.Minute)
if err != nil || !ok {
	return err // the backend is down, or another process runs the job: skip it
}
defer lease.Unlock(ctx)
```

| What | How |
|---|---|
| Returns | `ok` is false and `lease` is nil while someone else holds it |
| Errors | `ErrInvalidTTL` - the TTL is not positive. Backend errors |
| Redis | `SET key <random id> NX EX or PX` |

---

## `c.Lock`: wait for a lock { #clock }

Takes the lock, waiting while someone else holds it; use it for a lock that outlives one function,
otherwise use `WithLock`.

API: [`gashe.Cache.Lock`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Lock), [`gashe.Lease`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Lease)

```go
lease, err := c.Lock(ctx, "job:reindex", time.Minute)
defer lease.Unlock(ctx)
```

| What | How |
|---|---|
| Returns | a `*gashe.Lease` |
| Errors | `ctx.Err()` - `ctx` ended first. `ErrInvalidTTL` - the TTL is not positive. Backend errors |
| Redis | `SET key <random id> NX EX or PX`, every `LockPollInterval` while the lock is busy |

---

## `lease.Extend`: hold a lock longer { #leaseextend }

Gives the lock a new TTL from now, if it is still yours.

API: [`gashe.Lease.Extend`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Lease.Extend)

```go
err := lease.Extend(ctx, time.Minute)
```

| What | How |
|---|---|
| Errors | `ErrLockLost` - it expired, and someone else may hold it. `ErrInvalidTTL` - the TTL is not positive. Backend errors |
| Redis | a script that checks the holder id and runs `PEXPIRE` |

---

## `lease.Unlock`: release a lock { #leaseunlock }

Releases the lock if it is still yours; it ignores the cancellation of `ctx`, so a finished request
does not leave the lock behind.

API: [`gashe.Lease.Unlock`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Lease.Unlock)

```go
err := lease.Unlock(ctx)
```

| What | How |
|---|---|
| Errors | backend errors |
| Redis | a script that checks the holder id and runs `DEL` |

---

## `c.IsLocked`: check a lock { #cislocked }

Reports whether someone holds the lock.

API: [`gashe.Cache.IsLocked`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.IsLocked)

```go
locked, err := c.IsLocked(ctx, "job:reindex")
```

| What | How |
|---|---|
| Errors | backend errors |
| Redis | `EXISTS` |
