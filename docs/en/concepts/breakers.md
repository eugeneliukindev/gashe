# Circuit breakers { #breakers }

Stop calling a failing dependency for a while.

## `gashe.Guard`: stop calling a failing dependency { #gasheguard }

Runs one call through the circuit breaker of a key: a failing dependency is not called for a while.

API: [`gashe.Guard`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Guard), [`gashe.CircuitBreakerOptions`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#CircuitBreakerOptions)

```go
payments := gashe.CircuitBreakerOptions{
	FailurePercent: 50,
	MinCalls:       10,
	Window:         30 * time.Second,
	OpenFor:        15 * time.Second,
}
receipt, err := gashe.Guard(c, "payments-api", payments, func() (Receipt, error) {
	return provider.Charge(ctx, p)
})
```

```mermaid
stateDiagram-v2
    state "half-open" as half
    [*] --> closed
    closed --> open: FailurePercent of at least MinCalls failed within Window
    open --> half: after OpenFor, with HalfOpenCalls
    open --> closed: after OpenFor, without HalfOpenCalls
    half --> closed: a half-open call succeeds, or one Window passes
    half --> open: a half-open call fails
```

| State | Calls |
|---|---|
| closed | run and are counted |
| open | get `ErrCircuitOpen` and do not run |
| half-open | `HalfOpenCalls` half-open calls run; the rest get `ErrCircuitOpen` |

**Options** (`gashe.CircuitBreakerOptions`):

| Field | Type | Default | What it does |
|---|---|---|---|
| `FailurePercent` | `int`, 1 to 99 | required | opens once this share of calls failed |
| `MinCalls` | `int` | 0: 1 | calls the window needs before it may open |
| `Window` | `time.Duration` | required | the recent span calls are counted over, in 10 parts |
| `OpenFor` | `time.Duration` | required | how long the open breaker refuses calls |
| `HalfOpenCalls` | `int` | 0: it just closes | half-open calls after `OpenFor`: the first that succeeds closes the breaker, one that fails opens it again |
| `IsFailure` | `func(error) bool` | nil: any error | which errors count as failures |

| What | How |
|---|---|
| Returns | what the call returned |
| Errors | `ErrCircuitOpen` - the breaker is open, and the call did not run. `ErrInvalidOptions` - the options cannot work |
| Redis | none: the breaker lives in process memory, and each process judges by its own calls |
| Keys | the state of every key lives as long as the `Cache`: use a few fixed keys, not user ids |

- **Example:** [examples/breaker](https://github.com/eugeneliukindev/gashe/blob/main/examples/breaker/main.go)

???+ example "examples/breaker/main.go"

    ```go hl_lines="19 35 42 46"
    --8<-- "examples/breaker/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/breaker.txt"
    ```

---

## `c.CircuitBreaker`: a breaker for a call you report yourself { #ccircuitbreaker }

Checks the breaker before a call you report yourself; use it when the call does not fit one
function, otherwise use `Guard`.

API: [`gashe.Cache.CircuitBreaker`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.CircuitBreaker), [`gashe.DoneFunc`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#DoneFunc)

```go
done, err := c.CircuitBreaker("payments-api", payments)
if err != nil {
	return err
}
done(provider.Refund(ctx, p)) // once: nil or the error of the call
```

| What | How |
|---|---|
| Returns | a `DoneFunc` to call once with the result |
| Errors | as `gashe.Guard` |
| Redis | none |
