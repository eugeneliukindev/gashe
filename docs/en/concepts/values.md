# Values, keys and counters { #values-keys-and-counters }

## Values by key { #values-by-key }

The basic API: plain values, a TTL in every call, no `Policy`. It does not read entries of
`GetOrLoad`: keep one key on one side.

### `c.Set` { #cset }

```go
err := c.Set(ctx, "key", v, time.Hour) // ttl 0 = no expiry
```

Stores the value, replacing the entry and its TTL.

| What | How |
|---|---|
| Errors | `ErrInvalidTTL` - the TTL is negative; codec and backend errors |
| Redis | `SET`, with `EX` or `PX` for a TTL |

- **Example:** [examples/basic](https://github.com/eugeneliukindev/gashe/blob/main/examples/basic/main.go)

??? example "examples/basic/main.go"

    ```go
    --8<-- "examples/basic/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/basic.txt"
    ```

### `c.SetIfAbsent` { #csetifabsent }

```go
stored, err := c.SetIfAbsent(ctx, "key", v, time.Hour)
```

Stores the value only if the key has no entry.

| What | How |
|---|---|
| Returns | whether it stored |
| Errors | as `c.Set` |
| Redis | `SET ... NX PX`; `SETNX` without a TTL |

### `c.SetIfPresent` { #csetifpresent }

```go
stored, err := c.SetIfPresent(ctx, "key", v, time.Hour)
```

Stores the value only if the key has an entry.

| What | How |
|---|---|
| Returns | whether it stored |
| Errors | as `c.Set` |
| Redis | `SET ... XX` |

### `c.SetMany` { #csetmany }

```go
err := c.SetMany(ctx, map[string]any{"a": 1, "b": 2}, time.Hour)
```

Stores all values with one TTL.

| What | How |
|---|---|
| Errors | as `c.Set` |
| Redis | `SET` per key in one pipeline |

### `c.Get` { #cget }

```go
err := c.Get(ctx, "key", &dst)
```

Decodes the stored value into `dst`.

| What | How |
|---|---|
| Errors | `ErrNotFound` - a miss; a decode error; backend errors |
| Redis | `GET` |

### `c.GetMany` { #cgetmany }

```go
values, err := c.GetMany(ctx, "a", "b")
if values[0].Found() {
	err = values[0].Decode(&dst) // values[0].Key() is "a"
}
```

Reads the keys in one call.

| What | How |
|---|---|
| Returns | one `Value` per key, in order; a missing key gives a value that is not found |
| Errors | backend errors; `Decode` of a value not found returns `ErrNotFound` |
| Redis | `GET` per key in one pipeline |

### `c.Delete` { #cdelete }

```go
existed, err := c.Delete(ctx, "key")
```

Deletes the key as it is, without `Policy.Prefix`.

| What | How |
|---|---|
| Returns | whether the key existed |
| Redis | `UNLINK` |

### `c.DeleteMany` { #cdeletemany }

```go
err := c.DeleteMany(ctx, "a", "b")
```

Deletes the keys; missing keys are fine.

| What | How |
|---|---|
| Redis | `UNLINK` per key in one pipeline |


## Keys and counters { #keys-and-counters }

Patterns are Redis globs: `*`, `?`, `[abc]`, `[^a]`, `[a-z]`, `\` to escape.

### `c.Keys` { #ckeys }

```go
for key, err := range c.Keys(ctx, "user:*") {
	if err != nil {
		return err
	}
	fmt.Println(key) // without Options.Prefix
}
```

Yields every key that matches the pattern.

| What | How |
|---|---|
| Errors | a failure is yielded once, and the scan stops |
| Redis | `SCAN MATCH` on every Cluster master |
| Large backends | walks the whole database and may yield a key twice; use tags |

### `c.All` { #call }

```go
for v, err := range c.All(ctx, "user:*") {
	if err != nil {
		return err
	}
	_ = v.Decode(&dst) // v.Key() is the key
}
```

Yields every live value whose key matches the pattern.

| What | How |
|---|---|
| Errors | as `c.Keys` |
| Redis | `SCAN`, then `GET` of the found keys in pipelines of 100 |

### `c.DeleteMatch` { #cdeletematch }

```go
err := c.DeleteMatch(ctx, "user:*")
```

Deletes every key that matches the pattern, never past `Options.Prefix`.

| What | How |
|---|---|
| Redis | `SCAN`, then `UNLINK` in pipelines of 100 |
| Large backends | walks the whole database; use tags |

### `c.IncrBy` { #cincrby }

```go
n, err := c.IncrBy(ctx, "visits", 1, time.Hour) // by may be negative
```

Adds `by` to the integer under the key; creates it from 0.

| What | How |
|---|---|
| Returns | the new value |
| Errors | `ErrWrongType` - the value is not an integer. `ErrInvalidTTL` - the TTL is negative. An error on int64 overflow |
| Redis | `INCRBY`; with a TTL, `SET key 0 NX PX` and `INCRBY` in one `MULTI` |
| The TTL | applies only when the counter is created; later calls neither extend nor reset it |

### `c.Exists` { #cexists }

```go
ok, err := c.Exists(ctx, "key")
```

Reports whether the key has a live entry.

| What | How |
|---|---|
| Redis | `EXISTS` |

### `c.Expire` { #cexpire }

```go
existed, err := c.Expire(ctx, "key", time.Minute)
```

Sets a new TTL from now.

| What | How |
|---|---|
| Returns | whether the key existed |
| Errors | `ErrInvalidTTL` - the TTL is not positive |
| Redis | `PEXPIRE` |

### `c.TTL` { #cttl }

```go
left, err := c.TTL(ctx, "key")
```

Returns the time left; `gashe.NoExpiry` for a key without a TTL.

| What | How |
|---|---|
| Errors | `ErrNotFound` - the key is missing |
| Redis | `PTTL` |
