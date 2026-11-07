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
		Prefix: "orders:", // (1)!
		Hooks: gashe.Hooks{
			OnResult: onResult, // (2)!
			OnError:  onError,  // (3)!
		},
		CircuitBreaker:   cacheBreaker,          // stop calling a failing backend; zero means none
		Codec:            gashe.JSON{},          // the format of values; nil means JSON
		Compressor:       zstd.New(),            // shrinks what Codec produced; nil means none
		LockPollInterval: 20 * time.Millisecond, // (4)!
	})
	defer c.Shutdown(context.Background())
}
