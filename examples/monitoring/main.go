// Command monitoring counts how calls were answered and how long the loader ran, through
// Options.OnResult: the numbers for a hit ratio metric and for picking TTLs.
package main

import (
	"context"
	"fmt"
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
		OnError: func(_ context.Context, err error) { log.Println("cache degraded:", err) },
		OnResult: func(_ context.Context, o gashe.Outcome) {
			results[o.Result]++ // in production: a Prometheus counter by o.Result.String()
			loadTime += o.Load  // and a histogram of o.Load
		},
	})

	load := func(context.Context) (int, error) {
		time.Sleep(50 * time.Millisecond) // a query of 50ms
		return 1, nil
	}
	for _, key := range []string{"a", "a", "a", "b", "b", "c"} {
		gashe.GetOrLoad(ctx, c, key, gashe.Policy[int]{TTL: time.Minute}, load)
	}

	hits, misses := results[gashe.ResultHit], results[gashe.ResultMiss]
	fmt.Printf("hits %d, misses %d, hit ratio %d%%\n", hits, misses, 100*hits/(hits+misses))   // hits 3, misses 3, hit ratio 50%
	fmt.Println("three loads, 150ms or more in the loader:", loadTime >= 150*time.Millisecond) // true
}
