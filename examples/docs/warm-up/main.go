package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute) // a deadline for the waits
	defer cancel()
	mem := memory.New(memory.Options{MaxEntries: 100_000})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	products := gashe.For(c, productPolicy)
	query := func(_ context.Context, ids []string) (map[string]Product, error) { // SELECT ... WHERE id = ANY($1)
		out := make(map[string]Product, len(ids))
		for _, id := range ids {
			out[id] = Product{Price: 100}
		}
		return out, nil
	}
	hot := make([]string, 2000) // (1)!
	for i := range hot {
		hot[i] = strconv.Itoa(i)
	}
	for range 2 { // 20 queries of 100 ids; the second warm-up finds every id stored: none
		if err := warmUp(ctx, products, hot, query); err != nil {
			log.Fatal(err)
		}
	}
	refresh(ctx, products, hot[:8], func(context.Context, string) (Product, error) { // 0 failed
		return Product{Price: 120}, nil // SELECT ... WHERE id = $1
	})
	warmStale(ctx, c, hot[:100], query) // stale.go: a policy with Stale, warmed by a copy without it
}
