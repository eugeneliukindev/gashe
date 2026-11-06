# Not found { #negative }

Cache "not found", so lookups of missing ids stop reaching the source.

## `Negative`: cache "not found" { #negative-cache-not-found }

```go
gashe.Policy[User]{TTL: time.Hour, Negative: gashe.Negative{
	If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
	Err: sql.ErrNoRows,
	TTL: 30 * time.Second,
}}
```

Caches the loader error for `Negative.TTL` and returns `Negative.Err` meanwhile, so lookups of
missing ids stop reaching the database.

| Time | `GetOrLoad("999")` | `Result` |
|---|---|---|
| 0s | the loader: `sql.ErrNoRows`, `Negative.If` accepts it; "not found" stored for 30s | `miss` |
| 5s | "not found" is stored: `sql.ErrNoRows`, no query | `negative-hit` |
| 31s | expired: the loader runs again | `miss` |

| Field | What it does |
|---|---|
| `Negative.If` | which loader errors mean "not found"; nil turns `Negative` off |
| `Negative.Err` | the error returned while the "not found" is cached |
| `Negative.TTL` | how long the "not found" is cached |

!!! warning
    Data created meanwhile stays "not found" until `Negative.TTL` ends: keep it short, or delete the
    key when you create the data.

- **Example:** [examples/negative](https://github.com/eugeneliukindev/gashe/blob/main/examples/negative/main.go)

??? example "examples/negative/main.go"

    ```go
    --8<-- "examples/negative/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/negative.txt"
    ```
