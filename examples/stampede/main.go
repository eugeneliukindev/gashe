// Command stampede sends a thousand concurrent requests for one cold key: they share one load.
// With Redis and LockOnMiss, the key loads once for all processes, not once per process.
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	policy := gashe.Policy[string]{
		TTL:        time.Minute,
		LockOnMiss: 10 * time.Second, // over Redis: one load for all processes
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
	wg.Wait()
	fmt.Println("1000 requests, loads:", loads.Load()) // 1000 requests, loads: 1
}
