// Command basic shows the map-like API of gashe: Set, Get, GetMany, IncrBy and TTL.
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

	c.Set(ctx, "greeting", "hello", time.Minute)

	var s string
	c.Get(ctx, "greeting", &s) // s is "hello"
	c.Get(ctx, "missing", &s)  // gashe.ErrNotFound

	c.GetMany(ctx, "greeting", "missing") // greeting found, missing not found

	c.IncrBy(ctx, "visits", 1, time.Hour) // 1
	c.IncrBy(ctx, "visits", 1, time.Hour) // 2

	c.TTL(ctx, "greeting") // about a minute
}
