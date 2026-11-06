# Not found { #negative }

Cache "not found", so lookups of missing ids stop reaching the source.

## `Negative`: cache "not found" { #negative-cache-not-found }

Caches the loader error for `Negative.TTL` and returns `Negative.Err` meanwhile, so lookups of
missing ids stop reaching the database.

API: [`gashe.Policy`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Policy), [`gashe.Negative`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Negative)

```go
gashe.Policy[User]{TTL: time.Hour, Negative: gashe.Negative{
	If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
	Err: sql.ErrNoRows,
	TTL: 30 * time.Second,
}}
```

| Time | `GetOrLoad("999")` | `Result` |
|---|---|---|
| 0s | the loader: `sql.ErrNoRows`, `Negative.If` accepts it; "not found" stored for 30s | `miss` |
| 5s | "not found" is stored: `sql.ErrNoRows`, no query | `negative-hit` |
| 31s | expired: the loader runs again | `miss` |

**Options** (`gashe.Negative`):

| Field | Type | Default | What it does |
|---|---|---|---|
| `If` | `func(error) bool` | nil: off | which loader errors mean "not found" |
| `Err` | `error` | required with `If` | the error returned while the "not found" is cached, such as `sql.ErrNoRows` |
| `TTL` | `time.Duration` | required with `If` | how long the "not found" is cached; `Jitter` applies |

| What | How |
|---|---|
| With `StaleIfError` | a "not found" is an answer, not a failure: it is returned and stored, never hidden by the stale result |
| With `LockOnMiss` | processes that waited for the lock take the "not found" the holder stored |
| `Reload` | a "not found" leaves the stored result alone, as any error does |
| Not supported | `GetOrLoadMany` returns `ErrInvalidOptions`; `httpcache.New` panics |

!!! warning
    Data created meanwhile stays "not found" until `Negative.TTL` ends: keep it short, or delete the
    key when you create the data.

- **Example:** [examples/negative](https://github.com/eugeneliukindev/gashe/tree/main/examples/negative)

???+ example "examples/negative: without and with Negative"

    Three lookups of a user that does not exist. The two versions differ in the highlighted lines.

    === "With Negative"

        ```go hl_lines="8 26 27 28 29 30"

        --8<-- "examples/negative/with/main.go"

        ```

        Output: the first lookup caches the "not found", the other two get it from the cache: 1 query.

        ```text
        --8<-- "docs/snippets/examples/negative-with.txt"
        ```

    === "Without Negative"

        ```go

        --8<-- "examples/negative/without/main.go"

        ```

        Output: every lookup reaches the database, 3 queries.

        ```text
        --8<-- "docs/snippets/examples/negative-without.txt"
        ```
