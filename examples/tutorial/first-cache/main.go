// Command first-cache is step 1 of the tutorial: the loader runs once, then the cache answers.
package main

import (
	"context"
	"fmt"
	"log"
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
		OnResult: func(_ context.Context, o gashe.Outcome) { fmt.Printf("%s: %s\n", o.Key, o.Result) },
	})

	users := gashe.For(c, gashe.Policy[User]{TTL: time.Hour, Prefix: "user:"})

	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++ // SELECT * FROM users WHERE id = 42
		return User{ID: "42", Name: "Ann"}, nil
	}

	for range 3 {
		u, err := users.GetOrLoad(ctx, "42", loadUser)
		check(err)
		fmt.Println("  got", u.Name)
	}
	fmt.Println("database queries:", queries)
}

// check stops the program on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
