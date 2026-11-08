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
		Prefix:     "orders:",    // in front of every key, the own keys included; "" means none
		Codec:      gashe.JSON{}, // the format of values; nil means JSON
		Compressor: zstd.New(),   // shrinks what Codec produced; nil means none
		CircuitBreaker: gashe.CircuitBreaker{ // stops calling a failing backend; the zero value means none
			FailurePercent: 50,               // opens at this share of failed calls, 1 to 99; required
			MinCalls:       20,               // calls the window needs before it may open; 0 means 1
			Window:         10 * time.Second, // the span calls are counted over; required
			OpenFor:        5 * time.Second,  // how long the open breaker refuses calls; required
			HalfOpenCalls:  1,                // test calls after OpenFor; 0 means it just closes
			IsFailure: func(err error) bool { // which errors count; nil means any but a miss
				return !errors.Is(err, context.Canceled)
			},
		},
		LockPollInterval: 20 * time.Millisecond, // how often a busy lock is checked; 0 means 100ms
		Hooks: gashe.Hooks{
			OnResult: func(_ context.Context, o gashe.Outcome) { log.Println(o.Key, o.Result) }, // nil drops them
			OnError:  func(_ context.Context, err error) { log.Println("cache:", err) },         // nil drops them
		},
	})
	defer c.Shutdown(context.Background())
	// --8<-- [end:example]
}
