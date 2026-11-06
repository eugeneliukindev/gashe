# Rate limits { #rate-limits }

Refuse calls over a limit, counted across all pods.

## `c.RateLimit` { #cratelimit }

Counts one call for a key across all pods, and refuses calls over the limit.

```go
err := c.RateLimit(ctx, "login:"+ip, gashe.FixedWindow{
	Calls:  5,
	Per:    time.Minute,
	BanFor: 15 * time.Minute,
})
err = c.RateLimit(ctx, "api-key:"+key, gashe.PerMinute(600)) // = SlidingWindow{Calls: 600, Per: time.Minute}

var limited *gashe.RateLimitError // errors.Is(err, gashe.ErrRateLimited) holds too
if errors.As(err, &limited) {
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
	http.Error(w, "too many requests", http.StatusTooManyRequests)
}
```

**Default:** none: every call names its limit. Call it before the work it guards.

A refused call gets a `*RateLimitError` with `RetryAfter`; if the backend fails, the call is let
through and the error goes to `OnError`.

| Limit | How it counts |
|---|---|
| `FixedWindow` | a counter for the key starts with the first call and lives `Per`; the first call over `Calls` blocks the key for `BanFor`, counted from that call |
| `SlidingWindow` | a counter per span of `Per`; a call passes while the current one plus the last one, weighted by how much of it still falls inside the last `Per`, stays within `Calls`; a refused call is not counted |

**Options:**

| Limit | Field | What it does |
|---|---|---|
| `SlidingWindow` | `Calls`, `Per` | about `Calls` in any span of `Per`; also `PerSecond(n)`, `PerMinute(n)`, `PerHour(n)` |
| `FixedWindow` | `Calls`, `Per` | `Calls` per window of `Per` that starts with the first call |
| | `BanFor` | how long the first call over the limit blocks the key; 0 means `Per` |

| What | How |
|---|---|
| Errors | `*RateLimitError` - over the limit; it is `ErrRateLimited` and has `RetryAfter`. `ErrInvalidOptions` - the limit cannot work |
| Redis | `FixedWindow`: `INCRBY _rate:fixed:<key>` with the TTL `Per`, then `PEXPIRE` to `BanFor` or `PTTL`. `SlidingWindow`: `INCRBY` of the current span, `GET` of the last one, `INCRBY -1` when refused |
| `RetryAfter` | assumes no other calls come |
| Bursts | `SlidingWindow` is an estimate and can let slightly more through; `FixedWindow` can let twice `Calls` through at the border of two windows |

- **Example:** [examples/ratelimit](https://github.com/eugeneliukindev/gashe/blob/main/examples/ratelimit/main.go)

??? example "examples/ratelimit/main.go"

    ```go
    --8<-- "examples/ratelimit/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/ratelimit.txt"
    ```

### Requests and responses { #try-it-with-curl }

`/login` of [the example server](httpcache.md#try-it-with-curl) allows two calls a minute per
client with `FixedWindow{Calls: 2, Per: time.Minute, BanFor: 15 * time.Minute}`. Three calls:

| Call | Counter | Response |
|---|---|---|
| 1 | 1 of 2 | `200` |
| 2 | 2 of 2 | `200` |
| 3 | over the limit: banned for 15 minutes | `429`, `Retry-After: 900` |

```bash
--8<-- "docs/snippets/http/login-1.curl"
```

```http
--8<-- "docs/snippets/http/login-1.http"
```

The third call:

```bash
--8<-- "docs/snippets/http/login-3.curl"
```

```http
--8<-- "docs/snippets/http/login-3.http"
```
