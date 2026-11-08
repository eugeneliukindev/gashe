// Command for binds a policy to the cache once with gashe.For.
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

	// --8<-- [start:example]
	userCache := gashe.For(c, gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "users:v2:",
	})

	userCache.GetOrLoad(ctx, "42", func(context.Context) (User, error) {
		return User{Name: "Ann"}, nil
	})
	// --8<-- [end:example]
}
