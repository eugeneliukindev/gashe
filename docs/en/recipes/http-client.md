# Cache an HTTP API { #cache-an-http-api }

Cache the answers of an external HTTP API: the stale answer while it is down, a `404` for 10m.

## The code { #the-code }

Three files of one `package main`. The policy of `WeatherAPI`:

```go
--8<-- "examples/docs/http-client/api.go"
```

1. Never wait for the API forever.
2. The API is down: the last answer for 6 hours.
3. A 404 is asked again after 10 minutes, not on every call.

The loader turns every answer of the API into a result or an error:

```go
--8<-- "examples/docs/http-client/get.go"
```

`main` runs the client against a stand-in for the API, so the program runs as it is; in your code,
keep `WeatherAPI` and pass the real base URL.

```go
--8<-- "examples/docs/http-client/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/http-client/main.go"
    ```

## How it works { #how-it-works }

The loader turns every answer of the API into a result or an error; the policy decides by the error.

| The API answers | The loader returns | The cache |
|---|---|---|
| `200` | the `Weather` | stores it for `TTL` |
| `404` | `ErrNoCity` | caches it for [`Negative.For`](../reference/gashe.md#Negative): no call meanwhile; [not found](../concepts/negative.md#negative-cache-not-found) |
| `5xx`, `429`, a timeout, no connection | `errAPIDown` | the stale answer, up to `TTL + Stale.IfError.For` old, the error to [`OnError`](../reference/gashe.md#Hooks); [stale data](../concepts/stale.md#staleiferror-staleifslow) |
| anything else | another error | returns it, stores nothing |

| Part | What it does |
|---|---|
| [`Stale.IfError.IsFailure`](../reference/gashe.md#Stale) | only `errAPIDown` is a failure the stale answer covers; `ErrNoCity`, which [`Negative.IsNotFound`](../reference/gashe.md#Negative) accepts, never is: a city that is gone is not answered with a stale result |
| `http.Client.Timeout` | bounds the call; without it a hung API holds the request |

## Variations { #variations }

| If you need | Do |
|---|---|
| to answer at once when the API is slow | [`Stale.IfSlow`](../concepts/stale.md#stale-ifslow): `gashe.StaleIfSlow{For: 6 * time.Hour, After: 300 * time.Millisecond}`, the stale answer after 300ms, the call goes on in the background |
| the API to set the TTL, by `Cache-Control: max-age` | put the max-age into the result and use `TTL: func(w Weather) time.Duration { return w.MaxAge }`; [TTL](../concepts/loading.md#ttl) |
| to stay within the rate limit of the API | [`Stampede.LockFor`](../concepts/stampedes.md): one call per key for all processes; [`Stampede.MaxLoads`](../concepts/stampedes.md#maxconcurrentloads) <span class="version-badge">Added in v0.5</span>: at most N calls at once per process |
| popular keys never to wait | [`Stale.WhileRevalidate`](../concepts/stale.md#stalewhilerevalidate) |
| to cache the pages of your own server instead | [`httpcache`](../concepts/httpcache.md#httpcache-whole-http-responses) |
