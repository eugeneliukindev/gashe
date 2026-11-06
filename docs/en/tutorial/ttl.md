# 2. TTL { #ttl }

Goal: let a result expire, so a change in the database shows up.

## The program { #the-program }

```go
--8<-- "examples/tutorial/ttl/main.go"
```

## Output { #output }

```text
--8<-- "docs/snippets/examples/tutorial-ttl.txt"
```

## What happened { #what-happened }

- For its `TTL` a result is fresh: the second call is a `hit` with the old name, though the database
  has changed.
- After the `TTL` the result is gone: the next call is a `miss` and loads `Anna`.
- `Jitter: 0.1` makes each TTL up to 10% longer or shorter, so keys stored together do not expire
  together.

More: [TTL](../concepts/loading.md#ttl-how-long-a-result-is-fresh),
[Jitter](../concepts/stampedes.md#jitter-spread-expiry).

## Recap { #recap }

| Field | Does |
|---|---|
| `TTL` | how long a result is fresh: `time.Hour`, `"1h"` or a func of the result |
| `Jitter` | spreads each TTL by up to this share, from 0 to 1 |
