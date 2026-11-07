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
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})
	defer c.Shutdown(ctx) // waits for the background loads

	// The loader of each call stands for the database at that moment.
	v1 := func(context.Context) (string, error) { return "v1", nil }
	slowV2 := func(context.Context) (string, error) { time.Sleep(500 * time.Millisecond); return "v2", nil }
	v2 := func(context.Context) (string, error) { return "v2", nil }
	down := func(context.Context) (string, error) { return "", errors.New("database is down") }

	pages := gashe.For(c, gashe.Policy[string]{
		TTL: gashe.TTL{For: time.Second},
	})

	pages.GetOrLoad(ctx, "home", v1) // "v1", miss

	time.Sleep(1100 * time.Millisecond)  // past the TTL, the database is slow
	pages.GetOrLoad(ctx, "home", slowV2) // "v2" after 500ms, miss
	time.Sleep(600 * time.Millisecond)   // background loads are done
	pages.GetOrLoad(ctx, "home", v2)     // "v2", hit

	time.Sleep(1100 * time.Millisecond) // past the TTL, the database is down
	pages.GetOrLoad(ctx, "home", down)  // err: database is down, miss
}
