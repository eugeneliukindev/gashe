// Command prefix puts every key of the cache under Options.Prefix.
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

	c := gashe.New(backend, gashe.Options{Prefix: "orders:"}) // stores orders:user:42
	c.Set(ctx, "user:42", "Ann", time.Hour)
}
