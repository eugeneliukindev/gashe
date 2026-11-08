# Change a cached type { #change-a-cached-type }

Deploy a new version of a cached type: what gashe does by itself, and when to change [`Policy.Prefix`](../reference/gashe.md#Policy).

## The code { #the-code }

`UserV1` stands for the type before the deploy, `UserV2` for it after; in your code both are [`User`](../reference/httpcache.md#Vary).

```go
--8<-- "examples/docs/changing-types/main.go:example"
```

1. `{FullName:Ann Lee}`, a miss: the stored `UserV1` has another shape, and is never read as an
   empty `FullName`.
2. An old process during the rollout: a miss again, now the stored entry is a `UserV2`: each
   version overwrites the other.

??? example "Full program"

    ```go
    --8<-- "examples/docs/changing-types/main.go"
    ```

## How it works { #how-it-works }

Every entry keeps a hash of the form of its type: the exported field names, their `json` tags and
types, nested types included. An entry of another form is a miss, never a value with empty fields;
[type shapes](../concepts/loading.md#type-shapes).

| The change | What happens | Change `Prefix`? |
|---|---|---|
| add, remove, rename or reorder fields; change a field type or `json` tag | another shape: old entries are misses | only for a long rollout, see below |
| rename the struct type, add a method, change an unexported field | the same shape: old entries are read | no |
| the meaning, not the form: cents to dollars, new enum values | the same shape: old values read as new ones | yes |
| a `msgpack` tag, or a custom `MarshalJSON` or `MarshalMsgpack` | not in the shape | yes |
| [`Options.Codec`](../reference/gashe.md#Options) or [`Options.Compressor`](../reference/gashe.md#Options) | old entries of every type fail to decode: misses | no |

## During a rolling deploy { #during-a-rolling-deploy }

Old and new processes run side by side for the length of the rollout, over the same Redis.

| | The same `Prefix` | A new one, `users:v2:` |
|---|---|---|
| Misses | each version overwrites the entries of the other: misses all through the rollout | one per key, for the new version |
| Backend memory | one entry per key | two, until the old ones expire: `TTL` plus the stale window |
| [`Cached.Set`](../reference/gashe.md#Cached[V].Set), [`Cached.Delete`](../reference/gashe.md#Cached[V].Delete) of new processes | reach what old processes read | do not: old processes may answer stale results until the rollout ends |
| [`InvalidateTags`](../reference/gashe.md#Cache.InvalidateTags) | reaches both | reaches both: tags are not under `Prefix` |
| A rollback | misses again | the old entries are still there: hits |

## Variations { #variations }

| If you need | Do |
|---|---|
| to know the cost | one load per key read after the deploy, at most the hot keys of that type; without [`Stampede.LockFor`](../concepts/stampedes.md), once per process |
| to protect the database | [`Stampede.MaxLoads`](../concepts/stampedes.md#maxconcurrentloads) <span class="version-badge">Added in v0.5</span>, or fill the new prefix before the traffic: [warm up](warm-up.md#warm-up-the-cache) |
