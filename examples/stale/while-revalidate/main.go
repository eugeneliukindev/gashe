package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	// --8<-- [start:example]
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})
	defer c.Shutdown(ctx) // waits for the background loads

	// The loader of each call stands for the database at that moment.
	v1 := func(context.Context) (string, error) { return "v1", nil }
	slowV2 := func(context.Context) (string, error) { time.Sleep(500 * time.Millisecond); return "v2", nil }
	v2 := func(context.Context) (string, error) { return "v2", nil }
	down := func(context.Context) (string, error) { return "", errors.New("database is down") }

	pages := gashe.For(c, gashe.Policy[string]{
		TTL:   gashe.TTL{For: time.Second},
		Stale: gashe.Stale{WhileRevalidate: time.Minute},
	})

	fmt.Println(pages.GetOrLoad(ctx, "home", v1)) // miss

	time.Sleep(1100 * time.Millisecond)               // past the TTL, the database is slow
	fmt.Println(pages.GetOrLoad(ctx, "home", slowV2)) // at once, stale-while-revalidate
	time.Sleep(800 * time.Millisecond)                // background loads are done
	fmt.Println(pages.GetOrLoad(ctx, "home", v2))     // hit

	time.Sleep(1100 * time.Millisecond)             // past the TTL, the database is down
	fmt.Println(pages.GetOrLoad(ctx, "home", down)) // stale-while-revalidate
	// --8<-- [end:example]
}
