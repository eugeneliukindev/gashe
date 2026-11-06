# 4. Invalidation { #invalidation }

Goal: after a write to the database, stop answering the old result.

## The program { #the-program }

```go
--8<-- "examples/tutorial/invalidation/main.go"
```

## Output { #output }

```text
--8<-- "docs/snippets/examples/tutorial-invalidation.txt"
```

## What happened { #what-happened }

- Every user is stored with the tag `users`.
- `c.InvalidateTags(ctx, "users")` makes every entry with the tag a `miss`, `user:7` too: one write,
  nothing is deleted.
- Call it after the write, not before, or a read in between stores the old row again.
- `users.Set` stores a result you already have, with the TTL and tags of the policy: the next call
  is a `hit` with the new name.

More: [Invalidation](../concepts/invalidation.md).

## Recap { #recap }

| After a write | Use |
|---|---|
| many entries changed | `Tags` in the policy, then `c.InvalidateTags` |
| you have the new result | `users.Set(ctx, key, v)` |
