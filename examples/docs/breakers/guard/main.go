// Command guard runs a call to a payment provider through a circuit breaker.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type (
	Payment  struct{ Amount int }
	Receipt  struct{ ID string }
	Provider struct{}
)

func (Provider) Charge(context.Context, Payment) (Receipt, error) { return Receipt{ID: "r-1"}, nil }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	provider, p := Provider{}, Payment{Amount: 100}

	payments := gashe.CircuitBreaker{
		FailurePercent: 50,
		MinCalls:       10,
		Window:         30 * time.Second,
		OpenFor:        15 * time.Second,
	}
	receipt, err := gashe.Guard(c, "payments-api", payments, func() (Receipt, error) {
		return provider.Charge(ctx, p)
	})
	_, _ = receipt, err
}
