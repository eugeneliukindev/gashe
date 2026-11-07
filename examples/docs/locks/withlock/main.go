// Command withlock runs a job under a lock that one caller across all processes holds.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func reindex(context.Context) error { return nil } // the job

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	c.WithLock(ctx, "job:reindex", time.Minute, func(ctx context.Context) error {
		return reindex(ctx)
	}) // nil: the error of reindex
}
