// Command options sets every field of gashe.Options, nested ones included.
package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/compress/zstd"
)

func main() {
	backend := memory.New(memory.Options{})
	defer backend.Close()

	// --8<-- [start:example]
	c := gashe.New(backend, gashe.Options{
		// in front of every key, the own keys included; "" means none
		Prefix:     "orders:",
		Codec:      gashe.JSON{}, // the format of values; nil means JSON
		Compressor: zstd.New(),   // shrinks what Codec produced; nil means none
		// stops calling a failing backend; the zero value means none
		CircuitBreaker: gashe.CircuitBreaker{
			// opens at this share of failed calls, 1 to 99; required
			FailurePercent: 50,
			// calls the window needs before it may open; 0 means 1
			MinCalls: 20,
			// the span calls are counted over; required
			Window: 10 * time.Second,
			// how long the open breaker refuses calls; required
			OpenFor: 5 * time.Second,
			// test calls after OpenFor; 0 means it just closes
			HalfOpenCalls: 1,
			// which errors count; nil means any but a miss
			IsFailure: func(err error) bool {
				return !errors.Is(err, context.Canceled)
			},
		},
		// how often a busy lock is checked; 0 means 100ms
		LockPollInterval: 20 * time.Millisecond,
		Hooks: gashe.Hooks{
			// how each GetOrLoad was answered; nil drops it
			OnResult: func(_ context.Context, o gashe.Outcome) {
				log.Println(o.Key, o.Result)
			},
			// the errors the cache works around; nil drops them
			OnError: func(_ context.Context, err error) {
				log.Println("cache:", err)
			},
		},
	})
	defer c.Shutdown(context.Background())
	// --8<-- [end:example]
}
