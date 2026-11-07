// Command stampedes/lock-for: one process loads a missing key under a lock, the others wait for it.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Stats struct{ Orders int }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	statsCache := gashe.For(c, gashe.Policy[Stats]{
		TTL:      gashe.TTL{For: 10 * time.Minute},
		Stampede: gashe.Stampede{LockFor: time.Minute},
	})

	statsCache.GetOrLoad(ctx, "today", func(context.Context) (Stats, error) {
		return Stats{Orders: 120}, nil // the one query
	}) // {120}, miss
}
