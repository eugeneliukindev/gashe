// Command monitoring counts how calls were answered and how long the loader ran, through
// Options.OnResult: the numbers for a hit ratio metric and for picking TTLs.
package main

import (
	"context"
	"log"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()

	results := map[gashe.Result]int{}
	var loadTime time.Duration
	c := gashe.New(mem, gashe.Options{
		Hooks: gashe.Hooks{
			OnResult: func(_ context.Context, o gashe.Outcome) {
				results[o.Result]++ // in production: a counter by o.Result
				loadTime += o.Load  // and a histogram of o.Load
			},
			OnError: func(_ context.Context, err error) { log.Println("cache:", err) },
		},
	})

	load := func(context.Context) (int, error) {
		time.Sleep(50 * time.Millisecond) // a query of 50ms
		return 1, nil
	}
	for _, key := range []string{"a", "a", "a", "b", "b", "c"} {
		gashe.GetOrLoad(ctx, c, key, gashe.Policy[int]{TTL: gashe.TTL{For: time.Minute}}, load)
	}
	// results: 3 hits and 3 misses, a hit ratio of 50%; loadTime: 150ms or more
}
