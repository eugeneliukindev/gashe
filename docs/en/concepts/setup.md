# Setup { #setup }

## `gashe.New` { #gashenew }

Creates the cache over a backend; each option is explained below.

**Options below:** [`Prefix`](#prefix-several-services-in-one-backend),
[`Codec`](#codec-the-format-of-values), [`Compressor`](#compressor-large-values),
[`CircuitBreaker`](outages.md#circuitbreaker-keep-working-when-the-backend-is-down), [`OnResult`,
`OnError`](#onresult-onerror-monitoring).

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

| What | How |
|---|---|
| Errors | panics on a nil backend, a negative `LockPollInterval` or invalid `Options.CircuitBreaker` |
| Lifetime | create one at startup and share it; it does not close the backend |

### `Prefix`: several services in one backend { #prefix-several-services-in-one-backend }

```go
c := gashe.New(backend, gashe.Options{Prefix: "orders:"}) // stores orders:user:42
```

Puts every key of the cache under the prefix, own keys included; `Keys`, `All` and `DeleteMatch` never
reach past it. Keep environments in separate servers, not in the prefix. Key templates:
[keys](../internals/storage.md#keys).

---

### `Codec`: the format of values { #codec-the-format-of-values }

```go
gashe.JSON{}                      // the default, in the core: readable with redis-cli
msgpack.Codec{}                   // codec/msgpack, a separate module: binary, smaller, faster to decode
raw.Codec{Inner: msgpack.Codec{}} // codec/raw: []byte and string as they are, the rest to Inner
```

Turns a value into bytes and back; which to choose: [benchmarks](../internals/benchmarks.md#benchmarks).

| What | How |
|---|---|
| Changing it | makes every entry a miss at once |
| `raw.Codec` | helps the basic API only |
| `IncrBy` | does not work on MessagePack values |

---

### `Compressor`: large values { #compressor-large-values }

```go
zstd.New() // compress/zstd: a separate module
gzip.New() // compress/gzip: in the core module, the standard library only
```

Shrinks the bytes of the codec, with any codec, so big values take less memory in the backend and
less network on every read. Both store about the same, zstd is faster; which to choose:
[benchmarks](../internals/benchmarks.md#benchmarks).

| What | How |
|---|---|
| Lifetime | create one at startup and share it |
| Changing it | makes every entry a miss at once |
| `IncrBy` | does not work on compressed values |

---

### `CircuitBreaker`: keep working when the backend is down { #options-circuitbreaker }

See [Outages](outages.md#circuitbreaker-keep-working-when-the-backend-is-down).

### `OnResult`, `OnError`: monitoring { #onresult-onerror-monitoring }

```go
OnResult: func(ctx context.Context, o gashe.Outcome) {
	results.WithLabelValues(o.Result.String()).Inc() // hit ratio
	loads.Observe(o.Load.Seconds())                  // what a miss costs: pick TTLs by it
},
```

`OnResult` gets an `Outcome` for every `GetOrLoad` and every key of `GetOrLoadMany`; `OnError` gets
the errors the cache works around.

| What | How |
|---|---|
| `Outcome` | the key, how it was answered (hit, miss, stale-while-revalidate, stale-if-error, stale-if-slow, negative-hit, fallback), and how long the loader ran |
| Cost | both run synchronously on every call, so keep them fast |
| Scope | each sees its own process |

- **Example:** [examples/monitoring](https://github.com/eugeneliukindev/gashe/blob/main/examples/monitoring/main.go)

??? example "examples/monitoring/main.go"

    ```go
    --8<-- "examples/monitoring/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/monitoring.txt"
    ```

## `c.Shutdown` { #cshutdown }

```go
_ = srv.Shutdown(ctx) // the HTTP server first
_ = c.Shutdown(ctx)
_ = client.Close() // the backend last
```

Stops starting background reloads and waits for the running ones; call it before closing the
backend.

| What | How |
|---|---|
| Errors | `ctx.Err()` - `ctx` ended first |
| Afterwards | the cache keeps working, without background reloads |

## `c.Ping` { #cping }

```go
err := c.Ping(ctx)
```

Checks that the backend is reachable, past `Options.CircuitBreaker`.

| What | How |
|---|---|
| Redis | `PING` |

## `c.Backend` { #cbackend }

```go
b := c.Backend()
```

Returns the backend, with `Options.Prefix` and `Options.CircuitBreaker` applied.
