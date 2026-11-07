// Command quickstart: the loader runs once, then the cache answers.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct {
	ID   string
	Name string
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{}) // the backend: where entries live
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	users := gashe.For(c, gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "user:",
	})

	loadUser := func(context.Context) (User, error) {
		return User{ID: "42", Name: "Ann"}, nil // SELECT * FROM users WHERE id = 42
	}

	users.GetOrLoad(ctx, "42", loadUser) // Ann, miss: loaded and stored
	users.GetOrLoad(ctx, "42", loadUser) // Ann, hit: from the cache
}
