// Command basic shows the map-like API of gashe: Set, Get, GetMany, IncrBy and TTL.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	check(c.Set(ctx, "greeting", "hello", time.Minute))
	var s string
	check(c.Get(ctx, "greeting", &s))
	fmt.Println("Get greeting:", s) // Get greeting: hello

	if err := c.Get(ctx, "missing", &s); errors.Is(err, gashe.ErrNotFound) {
		fmt.Println("Get missing: gashe.ErrNotFound")
	}

	values, err := c.GetMany(ctx, "greeting", "missing")
	check(err)
	for _, v := range values {
		// GetMany greeting: found=true, then GetMany missing: found=false
		fmt.Printf("GetMany %s: found=%v\n", v.Key(), v.Found())
	}

	_, err = c.IncrBy(ctx, "visits", 1, time.Hour)
	check(err)
	visits, err := c.IncrBy(ctx, "visits", 1, time.Hour)
	check(err)
	fmt.Println("visits:", visits) // visits: 2

	left, err := c.TTL(ctx, "greeting")
	check(err)
	fmt.Println("greeting expires in", left.Round(time.Second)) // greeting expires in 1m0s
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
