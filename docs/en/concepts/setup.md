# Setup { #setup }

This page shows how to create the cache, watch how it answers and stop it. In this section, we will
go through [`gashe.New`](#gashenew) and its options, [`Cache.Shutdown`](#cshutdown),
[`Cache.Ping`](#cping) and [`Cache.Backend`](#cbackend).

## `gashe.New` { #gashenew }

You need one cache for the whole service. With `gashe.New`, you create it once at startup over a
backend and share it; each option has a card below.

??? api "API documentation"

    [`gashe.New`](../reference/gashe.md#New)<br>
    [`gashe.Options`](../reference/gashe.md#Options)<br>

```go hl_lines="27-38"
--8<-- "examples/docs/setup/options/main.go"
```

1. Goes before every key, the own keys of the cache included. `""` means none.
2. Gets how each `GetOrLoad` was answered. nil drops them.
3. Gets the errors the cache works around instead of returning them. nil drops them.
4. How often `Lock` and `Stampede.LockFor` check a busy lock. 0 means 100ms.

**Options** (`gashe.Options`):

| Field | Default | What it does |
|---|---|---|
| [`Prefix`](#prefix) | `""`: none | before every key the cache stores, its own keys included, separator included |
| [`Hooks`](#onresult-onerror) | none | `OnResult` gets how every call was answered, `OnError` the errors the cache works around |
| [`CircuitBreaker`](outages.md#circuitbreaker) | zero: none | stops calling a failing backend for a while |
| [`Codec`](#codec) | nil: JSON | the format of stored values |
| [`Compressor`](#compressor) | nil: none | shrinks what `Codec` produced |
| `LockPollInterval` | 0: 100ms | how often `Lock` and `Stampede.LockFor` try a busy lock again |

- `New` panics on a nil backend, a negative `LockPollInterval` or invalid `Options.CircuitBreaker`.
- The cache does not close the backend: you close it yourself, after [`Cache.Shutdown`](#cshutdown).

---

### `Options.Prefix` { #prefix }

Several services share one backend, and their keys must not collide. With `Prefix`, every key of
the cache, its own keys included, goes under the prefix, and `Keys`, `All` and `DeleteMatch` never
reach past it.

??? api "API documentation"

    [`gashe.Options`](../reference/gashe.md#Options)<br>

```go hl_lines="17"
--8<-- "examples/docs/setup/prefix/main.go"
```

- How each key is made: [keys](../internals/storage.md#keys).

!!! tip
    Keep environments, such as staging and production, in separate servers, not in the prefix.

!!! warning
    Your keys must not start with `__`: the own keys of the cache do.

---

### `Options.Codec` { #codec }

The backend stores bytes, not Go values. The `Codec` turns your value into bytes and back; JSON is
the default, and the [benchmarks](../internals/benchmarks.md#benchmarks) help you pick another.

??? api "API documentation"

    [`gashe.Codec`](../reference/gashe.md#Codec)<br>
    [`gashe.JSON`](../reference/gashe.md#JSON)<br>
    [`msgpack.Codec`](../reference/msgpack.md#Codec)<br>
    [`raw.Codec`](../reference/raw.md#Codec)<br>

```go hl_lines="22-24"
--8<-- "examples/docs/setup/codecs/main.go"
```

1. The default, in the core module: readable with `redis-cli`.
2. `codec/msgpack`, a separate module: binary, smaller, faster to decode.
3. `codec/raw`: `[]byte` and `string` as they are, the rest goes to `Inner`.

- `raw.Codec` helps the basic API only.
- `IncrBy` does not work on MessagePack values.

!!! warning
    Changing the codec makes every entry a miss at once: after the deploy, every key loads again.

---

### `Options.Compressor` { #compressor }

Large values take memory in the backend and network on every read. With a `Compressor`, you shrink
the bytes of any codec before they are stored.

??? api "API documentation"

    [`gashe.Compressor`](../reference/gashe.md#Compressor)<br>
    [`zstd.New`](../reference/zstd.md#New)<br>
    [`gzip.New`](../reference/gzip.md#New)<br>

```go hl_lines="21-22"
--8<-- "examples/docs/setup/compressors/main.go"
```

- `IncrBy` does not work on compressed values.
- Both store about the same, and zstd is faster: [benchmarks](../internals/benchmarks.md#benchmarks).

!!! tip
    Create one compressor at startup and share it.

!!! warning
    Changing the compressor makes every entry a miss at once: after the deploy, every key loads
    again.

---

### `Options.CircuitBreaker` { #circuitbreaker }

A backend that is down makes every call wait for its timeout. With `CircuitBreaker`, the cache stops
calling a failing backend for a while: see [Outages](outages.md#circuitbreaker).

??? api "API documentation"

    [`gashe.Options`](../reference/gashe.md#Options)<br>
    [`gashe.CircuitBreaker`](../reference/gashe.md#CircuitBreaker)<br>

---

### `Options.Hooks` { #onresult-onerror }

The cache hides many errors from you, and you cannot see your hit ratio. `OnResult` gets an
`Outcome` for every `GetOrLoad` call and every key of `GetOrLoadMany`; `OnError` gets the errors the
cache works around.

??? api "API documentation"

    [`gashe.Options`](../reference/gashe.md#Options)<br>
    [`gashe.Outcome`](../reference/gashe.md#Outcome)<br>

```go hl_lines="26-29"
--8<-- "examples/docs/setup/hooks/main.go"
```

1. What a miss costs: pick TTLs by it.

- Each sees its own process only: every process has its own memory, only the backend is shared.
- An `Outcome` holds the key, how it was answered (hit, miss, stale-while-revalidate,
  stale-if-error, stale-if-slow, negative-hit, fallback) and how long the loader ran.

!!! warning
    Both run synchronously on every call: a slow one, such as a network call, slows every
    `GetOrLoad`. Keep them fast.

```go hl_lines="22-29"
--8<-- "examples/monitoring/main.go"
```


---

## `Cache.Shutdown` { #cshutdown }

A background reload that is still running when you close the backend fails. `Cache.Shutdown` stops
starting background reloads and waits for the running ones; call it before you close the backend.

??? api "API documentation"

    [`Cache.Shutdown`](../reference/gashe.md#Cache.Shutdown)<br>

```go hl_lines="29-31"
--8<-- "examples/docs/setup/shutdown/main.go"
```

- It returns `ctx.Err()` if `ctx` ends first.
- Afterwards the cache keeps working, without background reloads.

---

## `Cache.Ping` { #cping }

Your health check must know whether the backend is reachable. `Cache.Ping` asks the backend itself,
past `Options.CircuitBreaker`.

??? api "API documentation"

    [`Cache.Ping`](../reference/gashe.md#Cache.Ping)<br>

```go hl_lines="18"
--8<-- "examples/docs/setup/ping/main.go"
```

- It returns backend errors.

??? note "Technical details"

    In Redis: `PING`.

---

## `Cache.Backend` { #cbackend }

Sometimes you need the backend as the cache sees it. `Cache.Backend` returns it with `Options.Prefix`
and `Options.CircuitBreaker` applied.

??? api "API documentation"

    [`Cache.Backend`](../reference/gashe.md#Cache.Backend)<br>

```go hl_lines="18"
--8<-- "examples/docs/setup/backend/main.go"
```
