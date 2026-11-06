# 7. HTTP pages { #http }

Goal: cache whole responses of a handler.

## The program { #the-program }

```go
--8<-- "examples/tutorial/http/main.go"
```

## Output { #output }

```text
--8<-- "docs/snippets/examples/tutorial-http.txt"
```

## What happened { #what-happened }

- `httpcache.New` takes the cache and a `Policy[httpcache.Response]`, and returns a middleware.
- The first `GET` runs the handler and stores its response: `fwd=uri-miss; stored`.
- The second is a `hit`: the stored status, headers and body; the handler ran once.
- Only `GET` and `HEAD` without `Authorization` and `Cookie` are cached, under the host, path and
  query.

More: [httpcache](../concepts/httpcache.md).

## Recap { #recap }

| Part | Does |
|---|---|
| `httpcache.New(c, httpcache.Options{Policy: ...})` | a middleware, `func(http.Handler) http.Handler` |
| `cached(handler)` | the handler with its responses cached |
| `Cache-Status` | how each response was answered |

That is the tutorial. Next: [Concepts](../concepts/setup.md), one page per problem.
