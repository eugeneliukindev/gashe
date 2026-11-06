# Change a cached type { #change-a-cached-type }

Deploy a new version of a cached type: what gashe does by itself, and when to change `Policy.Prefix`.

## The code { #the-code }

`UserV1` stands for the type before the deploy, `UserV2` for it after; in your code both are `User`.

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// UserV1 is the type before the deploy, UserV2 after it: one type in your code, two versions here.
type UserV1 struct{ Name string }
type UserV2 struct{ FullName string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{
		OnResult: func(_ context.Context, o gashe.Outcome) { fmt.Println(" ", o.Key, o.Result) },
	})

	fmt.Println("old pods:")
	v1 := gashe.For(c, gashe.Policy[UserV1]{TTL: time.Hour, Prefix: "users:"})
	_, _ = v1.GetOrLoad(ctx, "42", func(context.Context) (UserV1, error) { return UserV1{Name: "Ann"}, nil })
	_, _ = v1.GetOrLoad(ctx, "42", func(context.Context) (UserV1, error) { return UserV1{Name: "Ann"}, nil })

	fmt.Println("new pods, same prefix:") // another shape: a miss, not an empty FullName
	v2 := gashe.For(c, gashe.Policy[UserV2]{TTL: time.Hour, Prefix: "users:"})
	u, _ := v2.GetOrLoad(ctx, "42", func(context.Context) (UserV2, error) { return UserV2{FullName: "Ann Lee"}, nil })
	fmt.Printf("  %+v\n", u)

	fmt.Println("an old pod during the rollout:") // and back: each version overwrites the other
	_, _ = v1.GetOrLoad(ctx, "42", func(context.Context) (UserV1, error) { return UserV1{Name: "Ann"}, nil })

	fmt.Println("new pods with users:v2: the versions keep apart:")
	v2p := gashe.For(c, gashe.Policy[UserV2]{TTL: time.Hour, Prefix: "users:v2:"})
	for range 2 {
		_, _ = v2p.GetOrLoad(ctx, "42", func(context.Context) (UserV2, error) { return UserV2{FullName: "Ann Lee"}, nil })
	}
	_, _ = v1.GetOrLoad(ctx, "42", func(context.Context) (UserV1, error) { return UserV1{Name: "Ann"}, nil })
}
```

Output:

```text
old pods:
  users:42 miss
  users:42 hit
new pods, same prefix:
  users:42 miss
  {FullName:Ann Lee}
an old pod during the rollout:
  users:42 miss
new pods with users:v2: the versions keep apart:
  users:v2:42 miss
  users:v2:42 hit
  users:42 hit
```

## How it works { #how-it-works }

Every entry keeps a hash of the form of its type: the exported field names, their `json` tags and
types, nested types included. An entry of another form is a miss, never a value with empty fields;
[type shapes](../concepts/loading.md#type-shapes-type-changes-after-a-deploy).

| The change | What happens | Change `Prefix`? |
|---|---|---|
| add, remove, rename or reorder fields; change a field type or `json` tag | another shape: old entries are misses | only for a long rollout, see below |
| rename the struct type, add a method, change an unexported field | the same shape: old entries are read | no |
| the meaning, not the form: cents to dollars, new enum values | the same shape: old values read as new ones | yes |
| a `msgpack` tag, or a custom `MarshalJSON` or `MarshalMsgpack` | not in the shape | yes |
| `Options.Codec` or `Options.Compressor` | old entries of every type fail to decode: misses | no |

## During a rolling deploy { #during-a-rolling-deploy }

Old and new pods run side by side for the length of the rollout.

| | The same `Prefix` | A new one, `users:v2:` |
|---|---|---|
| Misses | each version overwrites the entries of the other: misses all through the rollout | one per key, for the new version |
| Backend memory | one entry per key | two, until the old ones expire: `TTL` plus the stale window |
| `Cached.Set`, `Cached.Delete` of new pods | reach what old pods read | do not: old pods may answer old data until the rollout ends |
| `InvalidateTags` | reaches both | reaches both: tags are not under `Prefix` |
| A rollback | misses again | the old entries are still there: hits |

- **The cost:** one load per key read after the deploy, at most the hot keys of that type; without
  [`LockOnMiss`](../concepts/stampedes.md#lockonmiss-one-load-for-all-pods), once per pod.
- **Protect the database:** [`MaxConcurrentLoads`](../concepts/stampedes.md#maxconcurrentloads-protect-the-source-from-too-many-loads-at-once),
  or fill the new prefix before the traffic: [warm up](warm-up.md#warm-up-the-cache).
