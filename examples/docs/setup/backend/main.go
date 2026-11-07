// Command backend reaches the backend through the cache, with Options.Prefix applied.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	backend := memory.New(memory.Options{})
	defer backend.Close()
	c := gashe.New(backend, gashe.Options{Prefix: "orders:"})

	b := c.Backend()
	b.Set(ctx, "user:42", []byte("Ann"), time.Hour) // stores orders:user:42
}
