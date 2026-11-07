// Command stale/while-revalidate: past the TTL the stale result answers at once, and a reload runs
// in the background.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	defer c.Shutdown(ctx) // waits for the background loads

	errDown := errors.New("database is down")

	// --8<-- [start:api]
	// The loader of each call stands for the database at that moment.
	v1 := func(context.Context) (string, error) { return "v1", nil }
	slowV2 := func(context.Context) (string, error) {
		time.Sleep(500 * time.Millisecond)
		return "v2", nil
	}
	v2 := func(context.Context) (string, error) { return "v2", nil }
	down := func(context.Context) (string, error) { return "", errDown }

	pages := gashe.For(c, gashe.Policy[string]{
		TTL: gashe.TTL{For: time.Second},
		Stale: gashe.Stale{
			WhileRevalidate: time.Minute,
		},
	})

	pages.GetOrLoad(ctx, "home", v1) // "v1", miss

	time.Sleep(1100 * time.Millisecond)  // past the TTL, the database is slow
	pages.GetOrLoad(ctx, "home", slowV2) // "v1" at once, stale-while-revalidate
	time.Sleep(600 * time.Millisecond)   // background loads are done
	pages.GetOrLoad(ctx, "home", v2)     // "v2", hit

	time.Sleep(1100 * time.Millisecond) // past the TTL, the database is down
	pages.GetOrLoad(ctx, "home", down)  // "v2", stale-while-revalidate
	// --8<-- [end:api]
}
