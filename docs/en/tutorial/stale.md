# 3. Stale data { #stale }

Goal: after the TTL, answer the old result instead of waiting for the database or failing with it.

## The program { #the-program }

```go
--8<-- "examples/tutorial/stale/main.go"
```

## Output { #output }

```text
--8<-- "docs/snippets/examples/tutorial-stale.txt"
```

## What happened { #what-happened }

- `users`, past the TTL: `stale-while-revalidate`, the old `Ann` at once, and a reload in the
  background.
- The next call is a `hit` with `Anna`, the result of that reload.
- `profiles`, past the TTL with the database down: the loader fails, and `stale-if-error` answers
  with the old result instead of the error.
- Each window is how long after the TTL the old result may answer; the entry is kept that long.
- `c.Shutdown` at the end waits for reloads still running in the background.

More: [Stale data](../concepts/stale.md).

## Recap { #recap }

| Field | The old result answers |
|---|---|
| `StaleWhileRevalidate` | at once, while one caller reloads it in the background |
| `StaleIfError` | when the loader fails |
