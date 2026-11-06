// Command breaker calls a payment API that starts failing: after enough failures the breaker
// opens and calls fail at once, then a test call closes it when the API works again.
package main

import (
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

	payments := gashe.CircuitBreakerOptions{
		FailurePercent: 50,
		MinCalls:       3,
		Window:         time.Minute,
		OpenFor:        300 * time.Millisecond,
		HalfOpenCalls:  1,
	}

	apiDown := true
	for n := 1; n <= 7; n++ {
		if n == 6 {
			fmt.Println("the API recovers; wait for OpenFor")
			apiDown = false
			time.Sleep(350 * time.Millisecond)
		}

		done, err := c.CircuitBreaker("payments-api", payments)
		if err != nil {
			fmt.Printf("call %d: %v, answered at once\n", n, err) // calls 4-5: gashe: circuit open
			continue
		}
		// Here the program calls the payment API, then tells the breaker how it went.
		if apiDown {
			done(errors.New("payment API timeout"))
			fmt.Printf("call %d: payment API timeout\n", n) // calls 1-3
			continue
		}
		done(nil)
		fmt.Printf("call %d: ok\n", n) // call 6 is the test call: it closes the breaker
	}
}
