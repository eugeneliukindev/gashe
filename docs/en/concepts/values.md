# Values, keys and counters { #values-keys-and-counters }

Store and read plain values by key, walk keys by pattern and keep counters, without a `Policy`.

## Values by key { #values-by-key }

The basic API: plain values, a TTL in every call, no `Policy`. It does not read entries of
`GetOrLoad`: keep one key on one side.

### `c.Set`: store a value { #cset }

Stores the value, replacing the entry and its TTL.

API: [`gashe.Cache.Set`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Set)

```go
err := c.Set(ctx, "key", v, time.Hour) // ttl 0 = no expiry
```

| What | How |
|---|---|
| Errors | `ErrInvalidTTL` - the TTL is negative; codec and backend errors, such as `memory.ErrTooLarge` |
| Redis | `SET`, with `EX` or `PX` for a TTL |

- **Example:** [examples/basic](https://github.com/eugeneliukindev/gashe/blob/main/examples/basic/main.go)

???+ example "examples/basic/main.go"

    ```go hl_lines="20 22"
    --8<-- "examples/basic/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/basic.txt"
    ```

---

### `c.SetIfAbsent`: store only a new key { #csetifabsent }

Stores the value only if the key has no entry.

API: [`gashe.Cache.SetIfAbsent`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.SetIfAbsent)

```go
stored, err := c.SetIfAbsent(ctx, "key", v, time.Hour)
```

| What | How |
|---|---|
| Returns | whether it stored |
| Errors | as `c.Set` |
| Redis | `SET ... NX EX or PX`; `SETNX` without a TTL |

---

### `c.SetIfPresent`: replace only an existing key { #csetifpresent }

Stores the value only if the key has an entry.

API: [`gashe.Cache.SetIfPresent`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.SetIfPresent)

```go
stored, err := c.SetIfPresent(ctx, "key", v, time.Hour)
```

| What | How |
|---|---|
| Returns | whether it stored |
| Errors | as `c.Set` |
| Redis | `SET ... XX`, with `EX` or `PX` for a TTL |

---

### `c.SetMany`: store many values { #csetmany }

Stores all values with one TTL.

API: [`gashe.Cache.SetMany`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.SetMany)

```go
err := c.SetMany(ctx, map[string]any{"a": 1, "b": 2}, time.Hour)
```

| What | How |
|---|---|
| Errors | as `c.Set` |
| Redis | `SET` per key in one pipeline |

---

### `c.Get`: read a value { #cget }

Decodes the stored value into `dst`.

API: [`gashe.Cache.Get`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Get)

```go
err := c.Get(ctx, "key", &dst)
```

| What | How |
|---|---|
| Errors | `ErrNotFound` - a miss. `ErrWrongType` - a Redis key that is not a string, such as a hash. A decode error; backend errors |
| Redis | `GET` |

---

### `c.GetMany`: read many values { #cgetmany }

Reads the keys in one call.

API: [`gashe.Cache.GetMany`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.GetMany), [`gashe.Value`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Value)

```go
values, err := c.GetMany(ctx, "a", "b")
if values[0].Found() {
	err = values[0].Decode(&dst) // values[0].Key() is "a"
}
```

| What | How |
|---|---|
| Returns | one `Value` per key, in order; a missing key, or a Redis key that is not a string, gives a value that is not found |
| Errors | backend errors; `Decode` of a value not found returns `ErrNotFound` |
| Redis | `GET` per key in one pipeline |

---

### `c.Delete`: delete a key { #cdelete }

Deletes the key as it is, without `Policy.Prefix`.

API: [`gashe.Cache.Delete`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Delete)

```go
existed, err := c.Delete(ctx, "key")
```

| What | How |
|---|---|
| Returns | whether the key existed |
| Errors | backend errors |
| Redis | `UNLINK` |

---

### `c.DeleteMany`: delete many keys { #cdeletemany }

Deletes the keys; missing keys are fine.

API: [`gashe.Cache.DeleteMany`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.DeleteMany)

```go
err := c.DeleteMany(ctx, "a", "b")
```

| What | How |
|---|---|
| Errors | backend errors |
| Redis | `UNLINK` per key in one pipeline |

---

## Keys and counters { #keys-and-counters }

Patterns are Redis globs: `*`, `?`, `[abc]`, `[^a]`, `[a-z]`, `\` to escape.

### `c.Keys`: walk keys by pattern { #ckeys }

Yields every key that matches the pattern.

API: [`gashe.Cache.Keys`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Keys)

```go
for key, err := range c.Keys(ctx, "user:*") {
	if err != nil {
		return err
	}
	fmt.Println(key) // without Options.Prefix
}
```

| What | How |
|---|---|
| Errors | a failure is yielded once, and the scan stops |
| Redis | `SCAN MATCH` on every Cluster master |
| Large backends | walks the whole database and may yield a key twice; use tags |

---

### `c.All`: walk values by pattern { #call }

Yields every live value whose key matches the pattern.

API: [`gashe.Cache.All`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.All), [`gashe.Value`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Value)

```go
for v, err := range c.All(ctx, "user:*") {
	if err != nil {
		return err
	}
	_ = v.Decode(&dst) // v.Key() is the key
}
```

| What | How |
|---|---|
| Errors | as `c.Keys` |
| Redis | `SCAN`, then `GET` of the found keys in pipelines of 100 |

---

### `c.DeleteMatch`: delete keys by pattern { #cdeletematch }

Deletes every key that matches the pattern, never past `Options.Prefix`.

API: [`gashe.Cache.DeleteMatch`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.DeleteMatch)

```go
err := c.DeleteMatch(ctx, "user:*")
```

| What | How |
|---|---|
| Errors | backend errors |
| Redis | `SCAN`, then `UNLINK` in pipelines of 100 |
| Large backends | walks the whole database; use tags |

---

### `c.IncrBy`: a counter { #cincrby }

Adds `by` to the integer under the key; creates it from 0.

API: [`gashe.Cache.IncrBy`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.IncrBy)

```go
n, err := c.IncrBy(ctx, "visits", 1, time.Hour) // by may be negative
```

| What | How |
|---|---|
| Returns | the new value |
| Errors | `ErrWrongType` - the value is not an integer. `ErrInvalidTTL` - the TTL is negative. An error on int64 overflow; backend errors |
| Redis | `INCRBY`; with a TTL, `SET key 0 NX EX or PX` and `INCRBY` in one `MULTI` |
| The TTL | applies only when the counter is created; later calls neither extend nor reset it |

---

### `c.Exists`: check a key { #cexists }

Reports whether the key has a live entry.

API: [`gashe.Cache.Exists`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Exists)

```go
ok, err := c.Exists(ctx, "key")
```

| What | How |
|---|---|
| Errors | backend errors |
| Redis | `EXISTS` |

---

### `c.Expire`: a new TTL { #cexpire }

Sets a new TTL from now.

API: [`gashe.Cache.Expire`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.Expire)

```go
existed, err := c.Expire(ctx, "key", time.Minute)
```

| What | How |
|---|---|
| Returns | whether the key existed |
| Errors | `ErrInvalidTTL` - the TTL is not positive; backend errors |
| Redis | `PEXPIRE` |

---

### `c.TTL`: the time a key has left { #cttl }

Returns the time left; `gashe.NoExpiry` for a key without a TTL.

API: [`gashe.Cache.TTL`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cache.TTL)

```go
left, err := c.TTL(ctx, "key")
```

| What | How |
|---|---|
| Errors | `ErrNotFound` - the key is missing; backend errors |
| Redis | `PTTL` |
