// Command quickstart: the loader runs once, then the cache answers.
package main

import (
	"context"
	"fmt"
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
	// --8<-- [start:example]
	mem := memory.New(memory.Options{}) // the backend: where entries live
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	users := gashe.For(c, gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "user:",
	})

	loadUser := func(context.Context) (User, error) {
		fmt.Println("SELECT * FROM users WHERE id = 42")
		return User{ID: "42", Name: "Ann"}, nil
	}

	fmt.Println(users.GetOrLoad(ctx, "42", loadUser)) // miss: loaded
	fmt.Println(users.GetOrLoad(ctx, "42", loadUser)) // hit
	// --8<-- [end:example]
}
