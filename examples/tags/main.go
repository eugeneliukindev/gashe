// Command tags invalidates everything cached about a user with one call after a write, and
// shows that the next read loads fresh data.
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

	name := "Ann" // the users table
	policy := gashe.Policy[string]{
		TTL:  gashe.TTL{For: time.Hour},
		Tags: []string{"user:42"}, // one tag per user
	}
	loadProfile := func(context.Context) (string, error) { return name, nil }

	gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile) // Ann: loaded
	gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile) // Ann: from the cache

	name = "Anna" // UPDATE users SET name = 'Anna' WHERE id = 42
	c.InvalidateTags(ctx, "user:42")
	gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile) // Anna: loaded again
}
