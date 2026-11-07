// Command stale/if-error: the stale rates answer an outage of the API for 6 hours after the TTL.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Rates map[string]float64

var ErrAPIDown = errors.New("rates API is down")

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	rateCache := gashe.For(c, gashe.Policy[Rates]{
		TTL: gashe.TTL{For: time.Minute},
		Stale: gashe.Stale{
			IfError: gashe.StaleIfError{
				For:       6 * time.Hour,
				IsFailure: func(err error) bool { return errors.Is(err, ErrAPIDown) }, // nil = any error
			},
		},
	})

	rateCache.GetOrLoad(ctx, "EUR", func(context.Context) (Rates, error) {
		return Rates{"USD": 1.08}, nil
	}) // {USD: 1.08}, miss
}
