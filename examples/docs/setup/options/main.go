// Command options creates the cache with every field of gashe.Options set.
package main

import (
	"context"
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
	onResult := func(_ context.Context, o gashe.Outcome) { log.Println(o.Key, o.Result) }
	onError := func(_ context.Context, err error) { log.Println("cache:", err) }
	cacheBreaker := gashe.CircuitBreaker{
		FailurePercent: 50,
		MinCalls:       20,
		Window:         10 * time.Second,
		OpenFor:        5 * time.Second,
	}

	c := gashe.New(backend, gashe.Options{
		// every field may be left out
		Prefix:           "orders:",             // (1)!
		Codec:            gashe.JSON{},          // the format of values; nil means JSON
		Compressor:       zstd.New(),            // shrinks what Codec produced; nil means none
		CircuitBreaker:   cacheBreaker,          // stop calling a failing backend; zero means none
		LockPollInterval: 20 * time.Millisecond, // (2)!
		Hooks: gashe.Hooks{
			OnResult: onResult, // (3)!
			OnError:  onError,  // (4)!
		},
	})
	defer c.Shutdown(context.Background())
	// --8<-- [end:example]
}
