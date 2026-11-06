# 5. Redis and many pods { #redis }

Goal: share the cache between pods, and load a cold key once for all of them.

## What changes { #what-changes }

Only the backend line. Add the module:

```bash
go get github.com/eugeneliukindev/gashe/backend/redis
```

and give `gashe.New` a Redis backend instead of `memory.New`:

```go
import (
	goredis "github.com/redis/go-redis/v9"

	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
defer client.Close()
c := gashe.New(gasheredis.New(client), gashe.Options{})
```

Every pod with the same Redis now sees the same entries, tags and locks.

## The program { #the-program }

Three pods ask for the same cold user at once. One memory backend shared by three caches stands in
for Redis, so the program runs without a server.

```go
--8<-- "examples/tutorial/redis/main.go"
```

## Output { #output }

```text
--8<-- "docs/snippets/examples/tutorial-redis.txt"
```

## What happened { #what-happened }

- Without `LockOnMiss` every pod misses and queries the database: 3 queries.
- With it, one pod takes a lock in the backend and loads; the others wait and read its result:
  1 query.
- Its value, here 5 seconds, is the longest the lock is held: make it longer than the slowest load.
- Calls within one pod share a load anyway; `LockOnMiss` is for many pods.

More: [Backends](../concepts/backends.md#backendredis),
[LockOnMiss](../concepts/stampedes.md#lockonmiss-one-load-for-all-pods).

## Recap { #recap }

| You want | Use |
|---|---|
| one cache for all pods | `gasheredis.New(client)` in place of `memory.New` |
| one load of a cold key for all pods | `LockOnMiss` in the policy |
