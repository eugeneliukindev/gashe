// Command tags invalidates every entry of a user with one tag, after the write to the database.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type (
	Profile struct{ Name string }
	Order   struct{ ID string }
)

func main() {
	ctx := context.Background()
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})

	profiles := gashe.For(c, gashe.Policy[Profile]{
		TTL:  gashe.TTL{For: time.Hour},
		Tags: []string{"user:42"},
	})
	orders := gashe.For(c, gashe.Policy[[]Order]{
		TTL:  gashe.TTL{For: time.Hour},
		Tags: []string{"user:42", "orders"},
	})
	_, _ = profiles, orders

	// db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = 42", "Ann")
	c.InvalidateTags(ctx, "user:42") // (1)!
}
