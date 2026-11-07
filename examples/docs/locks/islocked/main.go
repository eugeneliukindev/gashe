// Command islocked checks whether someone holds the lock, without taking it.
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

	c.IsLocked(ctx, "job:reindex") // true
	lease.Unlock(ctx)
	c.IsLocked(ctx, "job:reindex") // false
}
