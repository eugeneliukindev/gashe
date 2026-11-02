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

	var loads atomic.Int32
	report := func(context.Context) (string, error) {
		loads.Add(1)
		time.Sleep(200 * time.Millisecond) // a heavy aggregate query
		return "report", nil
	}

	var wg sync.WaitGroup
	for range 1000 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = gashe.GetOrLoad(ctx, c, "report:today",
				gashe.Policy[string]{TTL: time.Minute, LockOnMiss: 10 * time.Second}, report)
		}()
	}
	wg.Wait()
	fmt.Println("1000 requests, loads:", loads.Load()) // 1000 requests, loads: 1
}
