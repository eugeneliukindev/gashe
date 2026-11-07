// Command getorload reads a user through the cache, with gashe.For and without it.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	userPolicy := gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}}
	loadUser := func(context.Context) (User, error) {
		return User{Name: "Ann"}, nil // SELECT * FROM users WHERE id = 42
	}

	userCache := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}})
	userCache.GetOrLoad(ctx, "42", loadUser)            // Ann, miss
	gashe.GetOrLoad(ctx, c, "42", userPolicy, loadUser) // the same without For
}
