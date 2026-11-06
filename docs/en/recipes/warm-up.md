# Warm up the cache { #warm-up-the-cache }

Fill an empty cache, after a deploy or in a new region, before the traffic does it, without flooding
the database.

<span class="version-badge">Added in v0.5</span> `MaxConcurrentLoads` and `ErrOverloaded`.

## The code { #the-code }

```go hl_lines="26 40"
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Product struct {
	ID    string
	Price int64
}

var productPolicy = gashe.Policy[Product]{
	TTL:                time.Hour,
	Jitter:             0.2, // warmed together, they must not expire together
	MaxConcurrentLoads: 4,   // at most 4 queries at once in this process
	Prefix:             "product:",
}

// warmUp loads the hot ids the cache does not have yet, 100 per query.
func warmUp(ctx context.Context, products gashe.Cached[Product], ids []string,
	query func(ctx context.Context, ids []string) (map[string]Product, error),
) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(ids)/100+1)
	for batch := range slices.Chunk(ids, 100) {
		wg.Add(1)
		go func() { // MaxConcurrentLoads holds the rest back
			defer wg.Done()
			if _, err := products.GetOrLoadMany(ctx, batch, query); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	return <-errs // the first error, or nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) // a deadline for the waits
	defer cancel()
	mem := memory.New(memory.Options{MaxEntries: 100_000})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	products := gashe.For(c, productPolicy)

	var queries atomic.Int64
	query := func(_ context.Context, ids []string) (map[string]Product, error) { // SELECT ... WHERE id = ANY($1)
		queries.Add(1)
		time.Sleep(10 * time.Millisecond)
		out := make(map[string]Product, len(ids))
		for _, id := range ids {
			out[id] = Product{ID: id, Price: 100}
		}
		return out, nil
	}

	hot := make([]string, 2000) // in production: SELECT id FROM products ORDER BY views DESC LIMIT 2000
	for i := range hot {
		hot[i] = strconv.Itoa(i)
	}
	for range 2 {
		start := time.Now()
		if err := warmUp(ctx, products, hot, query); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("warm-up of %d ids: %d queries so far, took %v\n",
			len(hot), queries.Load(), time.Since(start).Round(10*time.Millisecond))
	}

	failed := refresh(ctx, products, hot[:8], func(ctx context.Context, id string) (Product, error) {
		queries.Add(1) // SELECT ... WHERE id = $1
		return Product{ID: id, Price: 120}, nil
	})
	p, err := products.GetOrLoad(ctx, "7", func(context.Context) (Product, error) {
		return Product{}, errors.New("not warmed") // a hit: never called
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("refresh of 8 ids: %d failed, %d queries so far, product 7 costs %d\n", failed, queries.Load(), p.Price)
}

// refresh reloads every id, whatever is stored: a job that runs before the entries expire.
func refresh(ctx context.Context, products gashe.Cached[Product], ids []string,
	queryOne func(ctx context.Context, id string) (Product, error),
) (failed int64) {
	var wg sync.WaitGroup
	var n atomic.Int64
	for _, id := range ids {
		wg.Add(1)
		go func() { // MaxConcurrentLoads holds the rest back
			defer wg.Done()
			_, err := products.Reload(ctx, id, func(ctx context.Context) (Product, error) { return queryOne(ctx, id) })
			if err != nil {
				n.Add(1) // the stored entry stays
			}
		}()
	}
	wg.Wait()
	return n.Load()
}
```

Output (times vary):

```text
warm-up of 2000 ids: 20 queries so far, took 50ms
warm-up of 2000 ids: 20 queries so far, took 0s
refresh of 8 ids: 0 failed, 28 queries so far, product 7 costs 120
```

## How it works { #how-it-works }

| Part | What it does |
|---|---|
| `GetOrLoadMany` per 100 ids | one query per batch, for the ids the cache does not have: a second run makes no query; [lists](../concepts/lists.md#getorloadmany) |
| `MaxConcurrentLoads: 4` | at most 4 queries at once in this process, whatever the number of goroutines; a batch takes one slot; [load limit](../concepts/stampedes.md#maxconcurrentloads-protect-the-source-from-too-many-loads-at-once) |
| `context.WithTimeout` | a batch that waits for a slot past the deadline gets `ErrOverloaded` instead of waiting forever |
| `Jitter: 0.2` | entries warmed together expire from 48m to 72m, not at once; [jitter](../concepts/stampedes.md#jitter-spread-expiry) |
| `Reload` in `refresh` | loads and stores every id, whatever is stored: a job that renews hot entries before they expire; [`Reload`](../concepts/invalidation.md#reload) |

## Variations { #variations }

| If you need | Do |
|---|---|
| a new region | point the warm-up at the cache of the region before it gets traffic; take the hot ids from the database or the logs of a live region |
| one warm-up for all processes | run it in one job, or in every process under [`c.TryLock`](../concepts/locks.md#ctrylock) |
| to warm a policy with `StaleIfError`, `StaleWhileRevalidate`, `LockOnMiss` or `Negative` | `GetOrLoadMany` returns `ErrInvalidOptions` for them: warm with a copy without them, as below (a second file of the same `package main`: it uses `Product`); `GetOrLoad` with the full policy reads what it stored |

```go
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
)

var stalePolicy = gashe.Policy[Product]{TTL: time.Hour, StaleIfError: time.Hour, LockOnMiss: 5 * time.Second}

func warmStale(ctx context.Context, c *gashe.Cache, batch []string,
	query func(ctx context.Context, ids []string) (map[string]Product, error),
) error {
	warm := stalePolicy // GetOrLoadMany refuses stale windows, LockOnMiss and Negative: a copy without them
	warm.StaleIfError, warm.StaleWhileRevalidate, warm.LockOnMiss, warm.Negative = 0, 0, 0, gashe.Negative{}
	_, err := gashe.GetOrLoadMany(ctx, c, batch, warm, query)
	return err
}
```

The warmed entries live for `TTL` only, without the stale window, until the next load stores them
with the full policy.
