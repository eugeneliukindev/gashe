# Setup { #setup }

Create the cache once at startup, watch how it answers, and stop it before the backend.

## `gashe.New`: create the cache { #gashenew }

Creates the cache over a backend; each option has a card below.

API: [`gashe.New`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#New), [`gashe.Options`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Options)

```go
c := gashe.New(backend, gashe.Options{ // every field may be left out
	Prefix:           "orders:",             // before every key, own keys included; "" means none
	Codec:            gashe.JSON{},          // the format of values; nil means JSON
	Compressor:       zstd.New(),            // shrinks what Codec produced; nil means none
	CircuitBreaker:   &cacheBreaker,         // stop calling a failing backend; nil means none
	OnError:          onError,               // errors worked around, not returned; nil drops them
	OnResult:         onResult,              // how each GetOrLoad was answered; nil drops them
	LockPollInterval: 20 * time.Millisecond, // how often Lock and LockOnMiss check; 0 means 100ms
})
```

**Options** (`gashe.Options`):

| Field | Type | Default | What it does |
|---|---|---|---|
| [`Prefix`](#prefix-several-services-in-one-backend) | `string` | `""`: none | before every key the cache stores, its own keys included, separator included |
| [`Codec`](#codec-the-format-of-values) | `gashe.Codec` | nil: JSON | the format of stored values |
| [`Compressor`](#compressor-large-values) | `gashe.Compressor` | nil: none | shrinks what `Codec` produced |
| [`CircuitBreaker`](outages.md#circuitbreaker-keep-working-when-the-backend-is-down) | `*gashe.CircuitBreakerOptions` | nil: none | stops calling a failing backend for a while |
| [`OnError`](#onresult-onerror-monitoring) | `func(context.Context, error)` | nil: dropped | gets the errors the cache works around instead of returning them |
| [`OnResult`](#onresult-onerror-monitoring) | `func(context.Context, gashe.Outcome)` | nil: dropped | gets how every `GetOrLoad` call, and every key of `GetOrLoadMany`, was answered |
| `LockPollInterval` | `time.Duration` | 0: 100ms | how often `Lock` and `LockOnMiss` try a busy lock again |

| What | How |
|---|---|
| Errors | panics on a nil backend, a negative `LockPollInterval` or invalid `Options.CircuitBreaker` |
| Lifetime | create one at startup and share it; it does not close the backend |

---

### `Prefix`: several services in one backend { #prefix-several-services-in-one-backend }

Puts every key of the cache under the prefix, own keys included; `Keys`, `All` and `DeleteMatch` never
reach past it.

API: [`gashe.Options`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Options)

```go
c := gashe.New(backend, gashe.Options{Prefix: "orders:"}) // stores orders:user:42
```

| What | How |
|---|---|
| Environments | keep them in separate servers, not in the prefix |
| Your keys | must not start with `__`: own keys of the cache do |
| Key templates | [keys](../internals/storage.md#keys) |

---

### `Codec`: the format of values { #codec-the-format-of-values }

Turns a value into bytes and back; which to choose: [benchmarks](../internals/benchmarks.md#benchmarks).

API: [`gashe.Codec`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Codec), [`gashe.JSON`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#JSON), [`msgpack.Codec`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/codec/msgpack#Codec), [`raw.Codec`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/codec/raw#Codec)

```go
gashe.JSON{}                      // the default, in the core: readable with redis-cli
msgpack.Codec{}                   // codec/msgpack, a separate module: binary, smaller, faster to decode
raw.Codec{Inner: msgpack.Codec{}} // codec/raw: []byte and string as they are, the rest to Inner
```

| What | How |
|---|---|
| Changing it | makes every entry a miss at once |
| `raw.Codec` | helps the basic API only |
| `IncrBy` | does not work on MessagePack values |

---

### `Compressor`: large values { #compressor-large-values }

Shrinks the bytes of the codec, with any codec, so big values take less memory in the backend and
less network on every read.

API: [`gashe.Compressor`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Compressor), [`zstd.New`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/compress/zstd#New), [`gzip.New`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/compress/gzip#New)

```go
zstd.New() // compress/zstd: a separate module
gzip.New() // compress/gzip: in the core module, the standard library only
```

| What | How |
|---|---|
| Which | both store about the same, zstd is faster: [benchmarks](../internals/benchmarks.md#benchmarks) |
| Lifetime | create one at startup and share it |
| Changing it | makes every entry a miss at once |
| `IncrBy` | does not work on compressed values |

---

### `CircuitBreaker`: keep working when the backend is down { #options-circuitbreaker }

Stops calling a failing backend for a while: see [Outages](outages.md#circuitbreaker-keep-working-when-the-backend-is-down).

API: [`gashe.Options`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Options), [`gashe.CircuitBreakerOptions`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#CircuitBreakerOptions)

---

### `OnResult`, `OnError`: monitoring { #onresult-onerror-monitoring }

`OnResult` gets an `Outcome` for every `GetOrLoad` and every key of `GetOrLoadMany`; `OnError` gets
the errors the cache works around.

API: [`gashe.Options`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Options), [`gashe.Outcome`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Outcome)

```go
OnResult: func(ctx context.Context, o gashe.Outcome) {
	results.WithLabelValues(o.Result.String()).Inc() // hit ratio
	loads.Observe(o.Load.Seconds())                  // what a miss costs: pick TTLs by it
},
```

| What | How |
|---|---|
| `Outcome` | the key, how it was answered (hit, miss, stale-while-revalidate, stale-if-error, stale-if-slow, negative-hit, fallback), and how long the loader ran |
| Cost | both run synchronously on every call, so keep them fast |
| Scope | each sees its own process: every process has its own memory, only the backend is shared |

- **Example:** [examples/monitoring](https://github.com/eugeneliukindev/gashe/blob/main/examples/monitoring/main.go)

???+ example "examples/monitoring/main.go"

    ```go hl_lines="23-26"
    --8<-- "examples/monitoring/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/monitoring.txt"
    ```

---

## `c.Shutdown`: stop background reloads { #cshutdown }

Stops starting background reloads and waits for the running ones; call it before closing the
backend.

API: [`gashe.Cache.Shutdown`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Shutdown)

```go
_ = srv.Shutdown(ctx) // the HTTP server first
_ = c.Shutdown(ctx)
_ = client.Close() // the backend last
```

| What | How |
|---|---|
| Errors | `ctx.Err()` - `ctx` ended first |
| Afterwards | the cache keeps working, without background reloads |

---

## `c.Ping`: a health check { #cping }

Checks that the backend is reachable, past `Options.CircuitBreaker`.

API: [`gashe.Cache.Ping`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Ping)

```go
err := c.Ping(ctx)
```

| What | How |
|---|---|
| Errors | backend errors |
| Redis | `PING` |

---

## `c.Backend`: the backend as the cache sees it { #cbackend }

Returns the backend, with `Options.Prefix` and `Options.CircuitBreaker` applied.

API: [`gashe.Cache.Backend`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Backend)

```go
b := c.Backend()
```
