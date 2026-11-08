// Command breaker calls a payment API that starts failing: after enough failures the breaker
// opens and calls fail at once, then a test call closes it when the API works again.
package main

import (
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:example]
	payments := gashe.CircuitBreaker{
		FailurePercent: 50,
		MinCalls:       3,
		Window:         time.Minute,
		OpenFor:        300 * time.Millisecond,
		HalfOpenCalls:  1,
	}

	// Calls 1-3: the payment API times out, and each call tells the breaker.
	for range 3 {
		done, _ := c.CircuitBreaker("payments-api", payments)
		done(errors.New("payment API timeout"))
	}

	// 3 of 3 calls failed: the breaker is open, and a call is refused at once.
	c.CircuitBreaker("payments-api", payments) // gashe.ErrCircuitOpen

	time.Sleep(350 * time.Millisecond) // past OpenFor, the API has recovered

	done, _ := c.CircuitBreaker("payments-api", payments) // a test call
	done(nil)                                             // it succeeds: closed
	// --8<-- [end:example]
}
