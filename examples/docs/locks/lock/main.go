// Command lock waits for the lock and releases it with the lease.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func runJob(ctx context.Context, c *gashe.Cache) error {
	lease, err := c.Lock(ctx, "job:reindex", time.Minute)
	if err != nil {
		return err // (1)!
	}
	defer lease.Unlock(ctx)
	return nil // the job
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	runJob(ctx, c)
}
