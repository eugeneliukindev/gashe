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

	// --8<-- [start:example]
	name := "Ann" // the users table
	policy := gashe.Policy[string]{
		TTL:  gashe.TTL{For: time.Hour},
		Tags: []string{"user:42"}, // one tag per user
	}
	loadProfile := func(context.Context) (string, error) {
		fmt.Println("SELECT name FROM users WHERE id = 42")
		return name, nil
	}

	fmt.Println(gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile))
	fmt.Println(gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile))

	name = "Anna" // UPDATE users SET name = 'Anna' WHERE id = 42
	c.InvalidateTags(ctx, "user:42")
	fmt.Println(gashe.GetOrLoad(ctx, c, "profile:42", policy, loadProfile))
	// --8<-- [end:example]
}
