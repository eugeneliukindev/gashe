# Loading { #loading }

How `GetOrLoad` answers, how long results are fresh and what gets stored.

## `gashe.For`: one policy per kind of data { #gashefor }

Binds a `Policy` to the cache and checks it once; keep one per kind of data.

API: [`gashe.For`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#For)

```go
userCache := gashe.For(c, gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"})
```

| What | How |
|---|---|
| Returns | a `gashe.Cached[User]`: the functions of the same name, with the policy; its `Set` and `Delete` apply `Policy.Prefix` |
| Errors | panics on an invalid policy, such as `TTL: "1hr"` |

---

## `GetOrLoad`: answer from the cache, load on a miss { #getorload }

Answers the stored result while the policy allows it; otherwise calls the loader, stores the result
and returns it.

API: [`gashe.Cached.GetOrLoad`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Cached.GetOrLoad), [`gashe.GetOrLoad`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#GetOrLoad)

```go
userCache := gashe.For(c, gashe.Policy[User]{TTL: time.Hour})
u, err := userCache.GetOrLoad(ctx, "42", loadUser)
u, err = gashe.GetOrLoad(ctx, c, "42", userPolicy, loadUser) // the same without For
```

Where the answer comes from; concurrent calls for one key and type in a process share one read and
one load.

```mermaid
flowchart TD
    G["GetOrLoad(key): read the entry and its tag counters"]
    G -- "the backend fails" --> F["Fallback, or the loader: nothing stored"]
    G -- "a cached not found" --> N[Negative.Err]
    G -- "fresh" --> H["the stored result"]
    G -- "within StaleWhileRevalidate" --> S["the stale result, a reload in the background"]
    G -- "a miss, an older tag or another shape" --> L["the loader, one process with LockOnMiss"]
```

**What a call answers** (the last column is the `Result` that `Options.OnResult` gets):

| The entry | The answer | The loader | `Result` |
|---|---|---|---|
| fresh, younger than `TTL` | the stored result | does not run | `hit` |
| stale, within `StaleWhileRevalidate` | the stale result | runs in the background, once for all processes | `stale-while-revalidate` |
| stale, within `StaleIfError`, the loader fails | the stale result | ran and failed; the error goes to `OnError` | `stale-if-error` |
| stale, within `StaleIfError`, the loader is slower than `StaleIfSlow` | the stale result | goes on in the background and stores its result | `stale-if-slow` |
| a "not found" within `Negative.TTL` | `Negative.Err` | does not run | `negative-hit` |
| missing or expired | the loaded result; an error is returned and not stored | runs | `miss` |
| the backend fails, `Fallback` has the key | the kept result | does not run | `fallback` |
| the backend fails, no `Fallback` | the loaded result | runs on every call | `miss` |

**Options** (`gashe.Policy[V]`; a zero field is off):

| Field | Type | Default | What it does |
|---|---|---|---|
| [`TTL`](#ttl-how-long-a-result-is-fresh) | `time.Duration` (`time.Hour`), a duration string for `time.ParseDuration` (`"1h30m"`), or `func(V) time.Duration` | required, unless a stale window is set | how long a result is fresh; a negative TTL from the function means "do not store" |
| [`Jitter`](stampedes.md#jitter-spread-expiry) | `float64`, 0 to less than 1 | 0: off | spreads every TTL randomly by up to this share: 0.1 is +-10% |
| [`StaleWhileRevalidate`](stale.md#stalewhilerevalidate-answer-old-data-while-reloading) | `time.Duration` | 0: off | how long after `TTL` the stale result is answered at once while one process reloads it in the background |
| [`StaleIfError`](stale.md#staleiferror-staleifslow-answer-old-data-while-the-source-fails) | `time.Duration` | 0: off | how long after `TTL` the stale result answers a loader error |
| `StaleIf` | `func(error) bool` | nil: any error | which loader errors `StaleIfError` covers |
| `StaleIfSlow` | `time.Duration` | 0: wait for the loader | how long a call waits for the loader before the stale result answers; needs `StaleIfError` |
| [`Negative`](negative.md#negative-cache-not-found) | `gashe.Negative` | off while `Negative.If` is nil | caches a "not found": `If` picks the errors, `Err` is returned, `TTL` keeps it |
| [`LockOnMiss`](stampedes.md#lockonmiss-one-load-for-all-processes) | `time.Duration` | 0: off | the longest the load lock is held: one process loads a miss, the others wait |
| [`MaxConcurrentLoads`](stampedes.md#maxconcurrentloads-protect-the-source-from-too-many-loads-at-once) | `int` | 0: no limit | how many loads of this kind of data run at once in one `*gashe.Cache` |
| [`Fallback`](outages.md#fallback-answer-from-memory-while-the-backend-is-down) | `*gashe.FallbackOptions` | nil: none | while the backend fails, keep and answer results in a backend of its own |
| [`Tags`](invalidation.md#tags-invalidate-many-entries-with-one-write) | `[]string` | nil: none | `InvalidateTags` of any of them makes the result a miss |
| [`CacheIf`](#cacheif-prefix-what-to-store-and-under-which-key) | `func(V) bool` | nil: every successful result | which results are stored |
| [`Prefix`](#cacheif-prefix-what-to-store-and-under-which-key) | `string` | `""`: none | before the key, after `Options.Prefix`, separator included |

| What | How |
|---|---|
| Default | with only `TTL`: a fresh result is a hit, a miss or an expired result calls the loader, a loader error is returned and not stored |
| Returns | the result as it reads back from the cache |
| Errors | the loader error, unless `StaleIfError` answers it. `Negative.Err` - a "not found" is cached. `ctx.Err()` - `ctx` ended while `LockOnMiss` waited. `ErrOverloaded` - every slot of `MaxConcurrentLoads` stayed taken until `ctx` ended. `ErrInvalidOptions` - an invalid policy, in the function form |
| Redis | `GET` of the key and its `__tag:` counters in one pipeline; `SET ... EX or PX` to store; `LockOnMiss` adds `SET __load:<key> NX EX or PX`, `StaleWhileRevalidate` adds `SET __refresh:<key> NX EX or PX` |
| The loader | gets a `ctx` of its own, with the values of the request but not its deadline; it is cancelled once every call waiting for it has left |
| [Type shapes](#type-shapes-type-changes-after-a-deploy) | an entry of another shape of `V` is a miss |
| `c.Get` | cannot read a `GetOrLoad` key: the basic API stores plain values |

- **Example:** [examples/cacheaside](https://github.com/eugeneliukindev/gashe/blob/main/examples/cacheaside/main.go)

???+ example "examples/cacheaside/main.go"

    ```go hl_lines="25-28 35"
    --8<-- "examples/cacheaside/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/cacheaside.txt"
    ```

Concurrent calls for one key in a process share one load:
[examples/stampede](https://github.com/eugeneliukindev/gashe/blob/main/examples/stampede/main.go)
sends a thousand at once.

???+ example "examples/stampede/main.go"

    ```go
    --8<-- "examples/stampede/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/stampede.txt"
    ```

---

## `TTL`: how long a result is fresh { #ttl-how-long-a-result-is-fresh }

A `time.Duration`, a string for `time.ParseDuration`, or a function of the result that gashe calls
after a load.

API: [`gashe.Policy`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Policy)

```go
TTL: time.Hour
TTL: "1h30m"
TTL: cfg.UserTTL // a string from the configuration, such as "10m"
TTL: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }
TTL: func(p Page) time.Duration {
	if p.Draft {
		return -1 // negative: do not store
	}
	return 10 * time.Minute
}
TTL: 3600 // an int, not a time.Duration: ErrInvalidOptions
```

| What | How |
|---|---|
| The entry lives | `TTL` plus the longer of `StaleWhileRevalidate` and `StaleIfError` |
| Depends on the arguments | needs no function: `TTL: ttlFor(itemID)` |
| Errors | `ErrInvalidOptions` - any other type, a string `time.ParseDuration` rejects, a negative duration, or neither `TTL` nor a stale window |

---

## `CacheIf`, `Prefix`: what to store and under which key { #cacheif-prefix-what-to-store-and-under-which-key }

`CacheIf` decides which results are stored; `Prefix` goes before every key of the policy.

API: [`gashe.Policy`](https://pkg.go.dev/github.com/eugeneliukindev/gashe#Policy)

```go
gashe.Policy[[]Item]{TTL: time.Minute, CacheIf: func(v []Item) bool { return len(v) > 0 }} // skip empty results
gashe.Policy[User]{TTL: time.Hour, Prefix: "users:v2:"}                                    // stores users:v2:42
```

| What | How |
|---|---|
| `CacheIf` false | the result is returned, not stored |
| Changing `Prefix` | makes every entry of that kind a miss at once |

---

## Type shapes: type changes after a deploy { #type-shapes-type-changes-after-a-deploy }

Built in: an entry of another type shape is a miss that loads again, so a renamed field does not
read as empty.

<!-- not compiled: two versions of one type -->
```go
type User struct{ Name string }     // before the deploy
type User struct{ FullName string } // after it: old entries are misses
```

| What | How |
|---|---|
| The shape | exported field names, `json` and `msgpack` tags and field types, nested types included |
| Values | do not change the shape: only changed types reload |
| Details | [type shapes](../internals/storage.md#type-shapes) |
