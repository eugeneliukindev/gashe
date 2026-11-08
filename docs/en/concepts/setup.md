# Create the cache { #setup }

This page shows how to create the cache, watch how it answers and stop it. In this section, we will
go through [`gashe.New`](#gashenew) and its options, [`Cache.Shutdown`](#cshutdown),
[`Cache.Ping`](#cping) and [`Cache.Backend`](#cbackend).

## Creating the cache { #gashenew }

You need one cache for the whole service. With [`gashe.New`](../reference/gashe.md#New), you create it once at startup over a
backend and share it; each option has a card below.

```go hl_lines="10-21"
--8<-- "examples/docs/setup/options/main.go:example"
```

1. Goes before every key, the own keys of the cache included. `""` means none.
2. How often [`Lock`](../reference/gashe.md#Cache.Lock) and [`Stampede.LockFor`](../reference/gashe.md#Stampede) check a busy lock. 0 means 100ms.
3. Gets how each [`GetOrLoad`](../reference/gashe.md#GetOrLoad) was answered. nil drops them.
4. Gets the errors the cache works around instead of returning them. nil drops them.

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/options/main.go"
    ```

**Signature:** `func New(b Backend, o Options) *Cache` · Every field: [`Options`](../reference/gashe.md#Options)

- `New` panics on a nil backend, a negative [`LockPollInterval`](../reference/gashe.md#Options) or invalid [`Options.CircuitBreaker`](../reference/gashe.md#Options).
- The cache does not close the backend: you close it yourself, after [`Cache.Shutdown`](#cshutdown).

---

### Key prefix { #prefix }

Several services share one backend, and their keys must not collide. With `Prefix`, every key of
the cache, its own keys included, goes under the prefix, and [`Keys`](../reference/gashe.md#Cache.Keys), `All` and [`DeleteMatch`](../reference/gashe.md#Cache.DeleteMatch) never
reach past it.

```go hl_lines="1"
--8<-- "examples/docs/setup/prefix/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/prefix/main.go"
    ```

- How each key is made: [keys](../internals/storage.md#keys).

!!! tip
    Keep environments, such as staging and production, in separate servers, not in the prefix.

!!! warning
    Your keys must not start with `__`: the own keys of the cache do.

---

### Value format { #codec }

The backend stores bytes, not Go values. The [`Codec`](../reference/gashe.md#Codec) turns your value into bytes and back; JSON is
the default, and the [benchmarks](../internals/benchmarks.md#benchmarks) help you pick another.

```go hl_lines="2-4"
--8<-- "examples/docs/setup/codecs/main.go:example"
```

1. The default, in the core module: readable with `redis-cli`.
2. [`codec/msgpack`](../reference/msgpack.md), a separate module: binary, smaller, faster to decode.
3. [`codec/raw`](../reference/raw.md): `[]byte` and `string` as they are, the rest goes to [`Inner`](../reference/raw.md#Codec).

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/codecs/main.go"
    ```

- [`raw.Codec`](../reference/raw.md#Codec) helps the basic API only.
- [`IncrBy`](../reference/gashe.md#Cache.IncrBy) does not work on MessagePack values.

!!! warning
    Changing the codec makes every entry a miss at once: after the deploy, every key loads again.

---

### Compression { #compressor }

Large values take memory in the backend and network on every read. With a [`Compressor`](../reference/gashe.md#Compressor), you shrink
the bytes of any codec before they are stored.

```go hl_lines="2-3"
--8<-- "examples/docs/setup/compressors/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/compressors/main.go"
    ```

- [`IncrBy`](../reference/gashe.md#Cache.IncrBy) does not work on compressed values.
- Both store about the same, and zstd is faster: [benchmarks](../internals/benchmarks.md#benchmarks).

!!! tip
    Create one compressor at startup and share it.

!!! warning
    Changing the compressor makes every entry a miss at once: after the deploy, every key loads
    again.

---

### A breaker for the backend { #circuitbreaker }

A backend that is down makes every call wait for its timeout. With [`CircuitBreaker`](../reference/gashe.md#CircuitBreaker), the cache stops
calling a failing backend for a while: see [When Redis is down](outages.md#circuitbreaker).

---

### Metrics and logs { #onresult-onerror }

The cache hides many errors from you, and you cannot see your hit ratio. With [`Options.Hooks`](../reference/gashe.md#Options),
[`OnResult`](../reference/gashe.md#Hooks) gets an [`Outcome`](../reference/gashe.md#Outcome) for every [`GetOrLoad`](../reference/gashe.md#GetOrLoad) call and every key of [`GetOrLoadMany`](../reference/gashe.md#GetOrLoadMany); [`OnError`](../reference/gashe.md#Hooks) gets the errors the
cache works around.

```go hl_lines="3-6"
--8<-- "examples/docs/setup/hooks/main.go:example"
```

1. What a miss costs: pick TTLs by it.

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/hooks/main.go"
    ```

- Each sees its own process only: every process has its own memory, only the backend is shared.
- An [`Outcome`](../reference/gashe.md#Outcome) holds the key, how it was answered (hit, miss, stale-while-revalidate,
  stale-if-error, stale-if-slow, negative-hit, fallback) and how long the loader ran.

!!! warning
    Both run synchronously on every call: a slow one, such as a network call, slows every
    [`GetOrLoad`](../reference/gashe.md#GetOrLoad). Keep them fast.

```go hl_lines="4-11"
--8<-- "examples/monitoring/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/monitoring/main.go"
    ```

---

## Stopping { #cshutdown }

A background reload that is still running when you close the backend fails. [`Cache.Shutdown`](../reference/gashe.md#Cache.Shutdown) stops
starting background reloads and waits for the running ones; call it before you close the backend.

```go hl_lines="12-14"
--8<-- "examples/docs/setup/shutdown/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/shutdown/main.go"
    ```

- It returns `ctx.Err()` if `ctx` ends first.
- Afterwards the cache keeps working, without background reloads.

---

## Health checks { #cping }

Your health check must know whether the backend is reachable. [`Cache.Ping`](../reference/gashe.md#Cache.Ping) asks the backend itself,
past [`Options.CircuitBreaker`](../reference/gashe.md#Options).

```go hl_lines="1"
--8<-- "examples/docs/setup/ping/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/ping/main.go"
    ```

- It returns backend errors.

??? note "Technical details"

    In Redis: `PING`.

---

## The backend underneath { #cbackend }

Sometimes you need the backend as the cache sees it. [`Cache.Backend`](../reference/gashe.md#Cache.Backend) returns it with [`Options.Prefix`](../reference/gashe.md#Options)
and [`Options.CircuitBreaker`](../reference/gashe.md#Options) applied.

```go hl_lines="3"
--8<-- "examples/docs/setup/backend/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/setup/backend/main.go"
    ```
