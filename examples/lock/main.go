// Command lock has three workers try to run the same job: the one that takes the lock runs it,
// the others skip. Over Redis the same holds for workers in different processes.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// Worker 1 comes first and takes the lock for a minute.
	lease, ok, _ := c.TryLock(ctx, "job:daily-report", time.Minute)
	fmt.Println("worker 1 took the lock:", ok) // true: it runs the report

	// Workers 2 and 3 come while worker 1 still runs the report.
	_, ok, _ = c.TryLock(ctx, "job:daily-report", time.Minute)
	fmt.Println("worker 2 took the lock:", ok) // false: it skips
	_, ok, _ = c.TryLock(ctx, "job:daily-report", time.Minute)
	fmt.Println("worker 3 took the lock:", ok) // false: it skips

	lease.Extend(ctx, time.Minute) // long work keeps the lock
	lease.Unlock(ctx)              // the report is done: the next run may take the lock
}
