// Command stale/if-slow: a call waits for the loader at most 200ms, then gets the stale rates.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Rates map[string]float64

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	defer c.Shutdown(ctx)

	rateCache := gashe.For(c, gashe.Policy[Rates]{
		TTL: gashe.TTL{For: time.Minute},
		Stale: gashe.Stale{
			IfSlow: gashe.StaleIfSlow{
				For:   time.Hour,
				After: 200 * time.Millisecond,
			},
		},
	})

	rateCache.GetOrLoad(ctx, "EUR", func(context.Context) (Rates, error) {
		return Rates{"USD": 1.08}, nil
	}) // {USD: 1.08}, miss
}
