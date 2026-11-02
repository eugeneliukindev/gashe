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
		OnResult: func(_ context.Context, o gashe.Outcome) {
			results[o.Result]++ // in production: a Prometheus counter by o.Result.String()
			loadTime += o.Load  // and a histogram of o.Load
		},
		OnError: func(_ context.Context, err error) { log.Println("cache degraded:", err) },
	})

	load := func(context.Context) (int, error) {
		time.Sleep(50 * time.Millisecond)
		return 1, nil
	}
	for _, key := range []string{"a", "a", "a", "b", "b", "c"} {
		_, err := gashe.GetOrLoad(ctx, c, key, gashe.Policy[int]{TTL: time.Minute}, load)
		check(err)
	}

	hits, misses := results[gashe.ResultHit], results[gashe.ResultMiss]
	// hits 3, misses 3, hit ratio 50%
	fmt.Printf("hits %d, misses %d, hit ratio %.0f%%\n", hits, misses, 100*float64(hits)/float64(hits+misses))
	fmt.Println("time spent in the loader:", loadTime.Round(time.Millisecond)) // about 150ms: three loads
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
