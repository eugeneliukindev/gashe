// Command stale/while-revalidate: past the TTL, the stale feed answers at once while one process
// reloads it.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Item struct{ Title string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	defer c.Shutdown(ctx)

	feedCache := gashe.For(c, gashe.Policy[[]Item]{
		TTL:   gashe.TTL{For: 5 * time.Minute},
		Stale: gashe.Stale{WhileRevalidate: 30 * time.Minute},
	})

	feedCache.GetOrLoad(ctx, "home", func(context.Context) ([]Item, error) {
		return []Item{{Title: "Hello"}}, nil
	}) // [Hello], miss
}
