// Command cacheaside caches a database call with GetOrLoad: the first call loads, the next ones
// are answered from the cache until TTL ends.
package main

import (
	"context"
	"fmt"
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

	policy := gashe.Policy[User]{
		TTL:    time.Minute,
		Prefix: "user:",
	}
	loadUser := func(context.Context) (User, error) {
		fmt.Println("load from the database") // SELECT ... WHERE id = 42
		return User{ID: 42, Name: "Ann"}, nil
	}

	for range 3 {
		u, _ := gashe.GetOrLoad(ctx, c, "42", policy, loadUser)
		fmt.Printf("got %+v\n", u) // three times; the database is asked only the first time
	}
}
