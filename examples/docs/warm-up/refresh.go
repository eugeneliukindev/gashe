package main

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/eugeneliukindev/gashe"
)

// refresh reloads every id, whatever is stored: a job that runs before the entries expire.
func refresh(ctx context.Context, products gashe.Cached[Product], ids []string,
	queryOne func(ctx context.Context, id string) (Product, error),
) (failed int64) {
	var wg sync.WaitGroup
	var n atomic.Int64
	for _, id := range ids {
		wg.Add(1)
		go func() { // Stampede.MaxLoads holds the rest back
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
