// Command tags invalidates everything cached about a user with one call after a write, and
// shows that the next read loads fresh data.
package main

import (
	"context"
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

	name := "Ann" // the users table
	policy := gashe.Policy[string]{
		TTL:  time.Hour,
		Tags: []string{"user:42"}, // one tag per user
	}
	loadProfile := func(context.Context) (string, error) {
		fmt.Println("  load from the database")
		return name, nil
	}

	fmt.Println("read twice:")
	p, _ := gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile)
	fmt.Println("  profile:", p) // loaded from the database
	p, _ = gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile)
	fmt.Println("  profile:", p) // from the cache

	fmt.Println("rename the user, then invalidate the tag:")
	name = "Anna" // UPDATE users SET name = 'Anna' WHERE id = 42
	c.InvalidateTags(ctx, "user:42")
	p, _ = gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile)
	fmt.Println("  profile:", p) // loaded again: Anna
}
