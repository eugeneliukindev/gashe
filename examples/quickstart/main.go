// Command quickstart is the program of the quickstart: the loader runs once, then the cache answers.
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
	mem := memory.New(memory.Options{}) // the backend: where entries live
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{
		// print how each call was answered
		OnResult: func(_ context.Context, o gashe.Outcome) { fmt.Printf("%s: %s\n", o.Key, o.Result) },
	})

	users := gashe.For(c, gashe.Policy[User]{
		TTL:    time.Hour,
		Prefix: "user:",
	})

	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++ // SELECT * FROM users WHERE id = 42
		return User{ID: "42", Name: "Ann"}, nil
	}

	for range 3 {
		u, _ := users.GetOrLoad(ctx, "42", loadUser)
		fmt.Println("  got", u.Name)
	}
	fmt.Println("database queries:", queries)
}
