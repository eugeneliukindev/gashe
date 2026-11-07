// Command extend gives a held lock a new TTL from now.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	lease, _ := c.Lock(ctx, "job:reindex", time.Minute)
	defer lease.Unlock(ctx)

	lease.Extend(ctx, time.Minute) // nil: the lock is still ours
}
