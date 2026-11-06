# 6. Outages { #outages }

Goal: keep answering, and keep the database safe, while Redis is down.

## The program { #the-program }

A memory backend that can fail stands in for Redis, so the program runs without a server.

```go
--8<-- "examples/tutorial/outages/main.go"
```

## Output { #output }

```text
--8<-- "docs/snippets/examples/tutorial-outages.txt"
```

## What happened { #what-happened }

- A failing Redis never fails `GetOrLoad`: the first call loads from the database, a `miss`.
- `Policy.Fallback` keeps that result in the memory of the pod, so the other 9 calls are a
  `fallback`, not a query: 1 query for 10 requests.
- `Options.CircuitBreaker` counts the failed calls; after 4 it opens and stops calling Redis, so no
  call waits for a timeout: 4 calls to Redis, 10 without the breaker.
- After `OpenFor` a call tries Redis again; once it answers, the fallback is no longer read.

More: [Outages](../concepts/outages.md).

## Recap { #recap }

| While Redis is down | Use |
|---|---|
| calls do not wait for its timeout | `Options.CircuitBreaker` |
| the database gets one load per key, not one per call | `Policy.Fallback` with a memory backend of its own |
