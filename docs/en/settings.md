# Settings { #settings }

Every field you can set, nested ones included, as you write them in code: each line says what
the field does and what leaving it out means. Every field may be left out unless it says
required. The programs compile in CI, so a renamed field breaks the build, not this page.

## Options { #options }

The options of a cache, given to [`gashe.New`](reference/gashe.md#New).

```go
--8<-- "examples/docs/settings/options/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/options/main.go"
    ```

Reference: [`Options`](reference/gashe.md#Options), [`CircuitBreaker`](reference/gashe.md#CircuitBreaker), [`Hooks`](reference/gashe.md#Hooks).

## Policy { #policy }

The rules of one kind of data, given to [`gashe.For`](reference/gashe.md#For),
[`GetOrLoad`](reference/gashe.md#GetOrLoad) and [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany).

```go
--8<-- "examples/docs/settings/policy/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/policy/main.go"
    ```

Reference: [`Policy`](reference/gashe.md#Policy), [`TTL`](reference/gashe.md#TTL), [`Stale`](reference/gashe.md#Stale), [`Stampede`](reference/gashe.md#Stampede), [`Negative`](reference/gashe.md#Negative), [`Fallback`](reference/gashe.md#Fallback), [`Store`](reference/gashe.md#Store).

## HTTP caching { #httpcache }

The options of the middleware, given to [`httpcache.New`](reference/httpcache.md#New).

```go
--8<-- "examples/docs/settings/httpcache/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/httpcache/main.go"
    ```

Reference: [`httpcache.Options`](reference/httpcache.md#Options), [`Vary`](reference/httpcache.md#Vary).

## Backends { #backends }

The options of the memory backend and of the near cache.

```go
--8<-- "examples/docs/settings/backends/main.go:example"
```

??? example "Full program"

    ```go
    --8<-- "examples/docs/settings/backends/main.go"
    ```

Reference: [`memory.Options`](reference/memory.md#Options), [`nearcache.Options`](reference/nearcache.md#Options). The Redis backend takes only its client: [`redis.New`](reference/redis.md#New).
