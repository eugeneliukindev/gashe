// Command trylock runs a job if the lock is free, and skips it otherwise.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func runJob(ctx context.Context, c *gashe.Cache) error {
	lease, ok, err := c.TryLock(ctx, "job:reindex", time.Minute)
	if err != nil || !ok {
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
