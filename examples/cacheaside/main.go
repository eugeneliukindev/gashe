// Command cacheaside caches a database call with GetOrLoad: the first call loads, the next ones
// are answered from the cache until TTL ends.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct {
	ID   int
	Name string
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:example]
	policy := gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Minute},
		Prefix: "user:",
	}
	loadUser := func(context.Context) (User, error) {
		return User{ID: 42, Name: "Ann"}, nil // SELECT ... WHERE id = 42
	}

	gashe.GetOrLoad(ctx, c, "42", policy, loadUser) // {42 Ann}: the loader runs
	gashe.GetOrLoad(ctx, c, "42", policy, loadUser) // {42 Ann}: from the cache
	gashe.GetOrLoad(ctx, c, "42", policy, loadUser) // {42 Ann}: from the cache
	// --8<-- [end:example]
}
