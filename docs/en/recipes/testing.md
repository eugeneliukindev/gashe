# Test code that uses gashe { #test-code-that-uses-gashe }

Test your own code that caches: an empty cache per test, TTLs in fake time, a backend that is down,
and your own backend.

## The code { #the-code }

The code under test, `users.go`, takes the cache and the query, so a test passes both:

```go
--8<-- "examples/docs/testing/users.go"
```

The tests, `users_test.go` in the same package: they use `User` and `New` from `users.go`.
`testing/synctest` needs Go 1.25 or newer. One query for many calls:

```go
--8<-- "examples/docs/testing/users_test.go"
```

1. No purge goroutine: nothing to close.

Expiry after the TTL, `ttl_test.go`:

```go
--8<-- "examples/docs/testing/ttl_test.go"
```

1. Fake time: the hour passes at once.

The cache backend is down, `down_test.go`:

```go
--8<-- "examples/docs/testing/down_test.go"
```

Your own backend, `backend_test.go`:

```go
--8<-- "examples/docs/testing/backend_test.go"
```

1. A backend must be safe for concurrent use.

## How it works { #how-it-works }

| Test | How |
|---|---|
| one query for many calls | `backend/memory` with `PurgeInterval: -1`: an empty cache per test, no goroutine to stop |
| expiry after `TTL` | `synctest.Test` runs in fake time: `time.Sleep(time.Hour + time.Second)` returns at once, and the memory backend sees the TTL pass |
| the cache backend is down | `downBackend`, the failing backend of [testing your code](../concepts/backends.md#testing-your-code): embed a real backend and override what must fail, here `Get`, `GetMany` and `Set`, so no read or write reaches it; `GetOrLoad` still answers from the loader |
| your own backend | `backendtest.Run` checks the contract the bundled backends pass; give it an empty backend per subtest; [your own backend](../concepts/backends.md#backendtestrun) |
| what the cache worked around | `OnError: t.Log` shows it in the output of a failed test |

## Variations { #variations }

| If you need | Do |
|---|---|
| to check what a `Stale.WhileRevalidate` reload stored | call `synctest.Wait()` inside the bubble first: it waits for the background reload |
| to check exact TTLs | leave `TTL.Jitter` out of the policy |
| only reads to fail | override `Get` and `GetMany` alone; writes then reach the real backend |
