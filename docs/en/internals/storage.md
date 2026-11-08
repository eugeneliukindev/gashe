# How gashe stores data { #how-gashe-stores-data }

What lands in the backend, for reading Redis with `redis-cli` and for reasoning about edge cases.

!!! warning
    The format of entries is internal: read it, `DEL` it, never write it by hand.

- [Keys](#keys), [TTL of an entry](#ttl-of-an-entry), [the GetOrLoad entry](#the-getorload-entry)
- [What each strategy stores](#what-each-strategy-stores), [tags](#tags), [type shapes](#type-shapes)
- [Known limits](#known-limits)

## Keys { #keys }

Parts are joined as they are, so put the separators into the prefixes yourself.

!!! warning
    Keys that start with `__` belong to the cache: keys of yours must not.

```text
<Options.Prefix><Policy.Prefix><key>             your keys; the basic API and locks have no Policy.Prefix
<Options.Prefix>__load:<Policy.Prefix><key>       the Stampede.LockFor lock of an entry
<Options.Prefix>__refresh:<Policy.Prefix><key>    the reload lock of an entry
<Options.Prefix>__tag:<tag>                       the counter of a tag, without Policy.Prefix
<Options.Prefix>__rate:fixed:<key>                a FixedWindow counter
<Options.Prefix>__rate:sliding:<key>:<span>       a SlidingWindow counter
```

---

## TTL of an entry { #ttl-of-an-entry }

| API | TTL |
|---|---|
| [`GetOrLoad`](../reference/gashe.md#GetOrLoad) | how long a result is fresh; the entry is kept for `TTL` plus the longest of [`Stale.WhileRevalidate`](../reference/gashe.md#Stale), [`Stale.IfError.For`](../reference/gashe.md#Stale) and [`Stale.IfSlow.For`](../reference/gashe.md#Stale), then the backend drops it |
| [`Negative`](../reference/gashe.md#Negative) | a "not found" lives [`Negative.For`](../reference/gashe.md#Negative), with [`TTL.Jitter`](../reference/gashe.md#TTL) |
| the basic API | 0 means "never expires", a negative one returns [`ErrInvalidTTL`](../reference/gashe.md#variables) |

---

## The GetOrLoad entry { #the-getorload-entry }

One string per key: an entry that the codec encodes, JSON by default.

```text
{"at":"2026-11-03T13:02:54.221386728+03:00","v":{"ID":42,"Name":"Ann"},"t":15130343655375510895}
```

| Field | JSON | `codec/msgpack` | What it is |
|---|---|---|---|
| when the result stops being fresh | `at` | `At` | a time |
| the result | `v` | `V` | the value |
| a cached "not found" | `neg`, only when `true` | `Neg`, always | `true` with [`Negative`](../reference/gashe.md#Negative) only |
| tag counters | `g`, only with [`Tags`](../reference/gashe.md#Policy) | `G`, always | the counters of its tags when it was stored |
| the shape | `t` | `T`, always | the shape of the type of `v`, see [type shapes](#type-shapes) |

The basic API stores the value itself, without this wrapper, so [`Cache.Get`](../reference/gashe.md#Cache.Get) cannot read a [`GetOrLoad`](../reference/gashe.md#GetOrLoad)
entry.

---

## What each strategy stores { #what-each-strategy-stores }

| Strategy | Key | Value | TTL |
|---|---|---|---|
| [`GetOrLoad`](../reference/gashe.md#GetOrLoad) | `user:42` | the entry above | `TTL` + the longer window |
| [`Negative`](../reference/gashe.md#Negative) | `user:999` | the entry above, with `neg` | [`Negative.For`](../reference/gashe.md#Negative) |
| [`Stale.WhileRevalidate`](../reference/gashe.md#Stale) reload | `__refresh:feed:u1` | the random id of the reloading process | `min(Stale.WhileRevalidate, 1 minute)`, deleted once stored |
| [`Stale.IfSlow`](../reference/gashe.md#Stale) load | `__refresh:feed:u1` | the random id of the loading process | 1 minute, deleted once stored |
| [`Stampede.LockFor`](../reference/gashe.md#Stampede) | `__load:stats:2026-11-02` | the random id of the loading process | [`Stampede.LockFor`](../reference/gashe.md#Stampede), deleted once stored |
| [`Lock`](../reference/gashe.md#Cache.Lock), [`TryLock`](../reference/gashe.md#Cache.TryLock), [`WithLock`](../reference/gashe.md#Cache.WithLock) | `job:reindex` | the random id of the holder | the lock TTL |
| [`Tags`](../reference/gashe.md#Policy) | `__tag:users` | a number | none |
| [`SlidingWindow`](../reference/gashe.md#SlidingWindow) | `__rate:sliding:api-key:k1:29893701` | the calls of one span of `Per` | twice `Per` |
| [`FixedWindow`](../reference/gashe.md#FixedWindow) | `__rate:fixed:login:1.2.3.4` | the calls of the window | `Per`; [`BanFor`](../reference/gashe.md#FixedWindow) after the first call over |
| [`CircuitBreaker`](../reference/gashe.md#CircuitBreaker) | nothing: it lives in process memory | | |

---

## Tags { #tags }

A tag is a counter: an entry keeps the counters of its tags from when it was stored, a lookup reads
them again, and an entry with an older counter is a miss.

| What | How |
|---|---|
| [`InvalidateTags`](../reference/gashe.md#Cache.InvalidateTags) | raises the counters: one write per tag, no scan, nothing deleted |
| A missing counter, never created or evicted | reads and [`InvalidateTags`](../reference/gashe.md#Cache.InvalidateTags) create it at a random generation; starting it at 0 or 1 would make entries invalidated before the eviction valid again |
| Order | a load reads the counters before it calls the loader, so a slow load cannot store a stale value after a write and [`InvalidateTags`](../reference/gashe.md#Cache.InvalidateTags) |

---

## Type shapes { #type-shapes }

`t` is a hash of the shape of the type: exported field names, `json` and `msgpack` tags and field
types, nested types included.

| What | How |
|---|---|
| Another shape | a miss that loads again, so a renamed field does not read as empty after a deploy |
| Values | do not change the shape |
| Entries without `t` | stored by gashe v0.1.0, read as they are |

---

## Known limits { #known-limits }

| Limit | What happens |
|---|---|
| The race of `Delete` and [`Cached.Set`](../reference/gashe.md#Cached[V].Set) | a load reads the stale value, a write updates the database and deletes or sets the key, then the slower load stores its stale value until its TTL ends; only tags do not have this race |
| Locks are advisory | a holder paused longer than the TTL (GC, a frozen VM) can overlap with the next one |
| Clocks | freshness compares the clock of the reading process with the time the writer stored: keep clocks in sync (NTP) |
| One key, one type | two call sites that read one key as different types make each other miss on every call: give every type its own key or `Prefix` |
