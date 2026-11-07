package main

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/eugeneliukindev/gashe"
)

type Product struct{ Price int64 }

var productPolicy = gashe.Policy[Product]{
	TTL:      gashe.TTL{For: time.Hour, Jitter: 0.2}, // warmed together, they must not expire together
	Prefix:   "product:",
	Stampede: gashe.Stampede{MaxLoads: 4}, // at most 4 queries at once in this process
}

// warmUp loads the hot ids the cache does not have yet, 100 per query.
func warmUp(ctx context.Context, products gashe.Cached[Product], ids []string,
	query func(ctx context.Context, ids []string) (map[string]Product, error),
) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(ids)/100+1)
	for batch := range slices.Chunk(ids, 100) {
		wg.Go(func() { // (1)!
			if _, err := products.GetOrLoadMany(ctx, batch, query); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	return <-errs // the first error, or nil
}
