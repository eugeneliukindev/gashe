// Command keys-counters walks the keys that match a pattern and counts visits.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	if err := run(context.Background()); err != nil {
		panic(err)
	}
}

func run(ctx context.Context) error {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	for key, err := range c.Keys(ctx, "user:*") {
		if err != nil {
			return err
		}
		fmt.Println(key) // without Options.Prefix
	}
	n, err := c.IncrBy(ctx, "visits", 1, time.Hour) // by may be negative
	_ = n
	return err
}
