// Command stampede: a thousand concurrent requests for one cold key share one load.
package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})

	policy := gashe.Policy[string]{
		TTL:      gashe.TTL{For: time.Minute},
		Stampede: gashe.Stampede{SingleFlight: true}, // concurrent calls share one load
	}
	var loads atomic.Int32 // the requests run at once
	loadReport := func(context.Context) (string, error) {
		loads.Add(1)
		time.Sleep(200 * time.Millisecond) // a heavy aggregate query
		return "report", nil
	}

	var wg sync.WaitGroup
	for range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gashe.GetOrLoad(ctx, c, "report:today", policy, loadReport)
		}()
	}
	wg.Wait() // loads is 1: a thousand calls share one load
}
