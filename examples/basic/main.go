// Command basic shows the map-like API of gashe: Set, Get, GetMany, IncrBy and TTL.
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
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	c.Set(ctx, "greeting", "hello", time.Minute)
	var s string
	c.Get(ctx, "greeting", &s)
	fmt.Println("Get greeting:", s) // Get greeting: hello

	err := c.Get(ctx, "missing", &s)
	if errors.Is(err, gashe.ErrNotFound) {
		fmt.Println("Get missing: gashe.ErrNotFound")
	}

	values, _ := c.GetMany(ctx, "greeting", "missing")
	for _, v := range values {
		fmt.Printf("GetMany %s: found=%v\n", v.Key(), v.Found()) // greeting: found=true, missing: found=false
	}

	c.IncrBy(ctx, "visits", 1, time.Hour)
	visits, _ := c.IncrBy(ctx, "visits", 1, time.Hour)
	fmt.Println("visits:", visits) // visits: 2

	left, _ := c.TTL(ctx, "greeting")
	fmt.Println("greeting expires in", left.Round(time.Second)) // greeting expires in 1m0s
}
