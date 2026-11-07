// Command check-and-report checks the breaker before a call and reports the result of the call.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type (
	Payment  struct{ Amount int }
	Provider struct{}
)

func (Provider) Refund(context.Context, Payment) error { return nil }

var payments = gashe.CircuitBreaker{
	FailurePercent: 50,
	MinCalls:       10,
	Window:         30 * time.Second,
	OpenFor:        15 * time.Second,
}

func refund(ctx context.Context, c *gashe.Cache, provider Provider, p Payment) error {
	done, err := c.CircuitBreaker("payments-api", payments)
	if err != nil {
		return err
	}
	done(provider.Refund(ctx, p)) // (1)!
	return nil
}

func main() {
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})

	refund(context.Background(), c, Provider{}, Payment{Amount: 100})
}
