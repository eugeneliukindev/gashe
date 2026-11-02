// Command lock runs a job from three workers at once: one takes the lock and runs it, the others
// skip. Over Redis the same holds for workers in different processes.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	var wg sync.WaitGroup
	for worker := 1; worker <= 3; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, ok, err := c.TryLock(ctx, "job:daily-report", time.Minute)
			check(err)
			if !ok {
				fmt.Printf("worker %d: someone else runs the report, skip\n", worker) // two of the three workers
				return
			}
			defer func() { _ = lease.Unlock(ctx) }()
			fmt.Printf("worker %d: runs the report\n", worker) // one worker, such as worker 3
			time.Sleep(200 * time.Millisecond)
			check(lease.Extend(ctx, time.Minute)) // long work keeps the lock
		}()
	}
	wg.Wait()
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
