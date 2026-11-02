// Command breaker calls a payment API that starts failing: after enough failures the breaker
// opens and calls fail at once, then a test call closes it when the API works again.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	down := true
	charge := func(context.Context) error {
		if down {
			return errors.New("payment API timeout")
		}
		return nil
	}
	payments := gashe.CircuitBreakerOptions{
		FailurePercent: 50, MinCalls: 3, Window: time.Minute, OpenFor: 300 * time.Millisecond, HalfOpenCalls: 1,
	}

	call := func(n int) {
		done, err := c.CircuitBreaker("payments-api", payments)
		if err != nil {
			fmt.Printf("call %d: %v, answered at once\n", n, err)
			return
		}
		err = charge(context.Background())
		done(err)
		fmt.Printf("call %d: %v\n", n, describe(err))
	}

	// calls 1-3: payment API timeout; 4-5: gashe: circuit open, answered at once
	for n := 1; n <= 5; n++ {
		call(n)
	}
	fmt.Println("the API recovers; wait for OpenFor")
	down = false
	time.Sleep(350 * time.Millisecond)
	call(6) // call 6: ok, the test call closes the breaker
	call(7) // call 7: ok
}

func describe(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}
