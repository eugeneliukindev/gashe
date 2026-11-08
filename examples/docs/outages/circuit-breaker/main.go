// Command circuit-breaker: the cache stops calling a failing backend for a while.
package main

import (
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()

	// --8<-- [start:example]
	cacheBreaker := gashe.CircuitBreaker{
		FailurePercent: 50,
		MinCalls:       20,
		Window:         10 * time.Second,
		OpenFor:        5 * time.Second,
	}
	_ = gashe.New(mem, gashe.Options{CircuitBreaker: cacheBreaker})
	// --8<-- [end:example]
}
