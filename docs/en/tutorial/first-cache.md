# 1. First cache { #first-cache }

Goal: load a user once, then answer from the cache.

## The program { #the-program }

```go
--8<-- "examples/tutorial/first-cache/main.go"
```

## Output { #output }

```text
--8<-- "docs/snippets/examples/tutorial-first-cache.txt"
```

## What happened { #what-happened }

- `memory.New` is the backend: the place entries live, here the memory of the process.
- `gashe.New` makes the cache over it; `OnResult` prints how each call was answered.
- `gashe.For` binds a `Policy` to the cache: the type `User`, a `TTL` of an hour, the key prefix
  `user:`.
- The first `GetOrLoad` is a `miss`: the loader runs and its result is stored under `user:42`.
- The next two are a `hit`: the stored result, no query.

More: [Setup](../concepts/setup.md), [Loading](../concepts/loading.md).

## Recap { #recap }

| Call | Does |
|---|---|
| `gashe.New(backend, gashe.Options{})` | one cache for the whole program |
| `gashe.For(c, gashe.Policy[V]{...})` | one per kind of data, kept and shared |
| `users.GetOrLoad(ctx, key, load)` | the stored result, or the loader once |
