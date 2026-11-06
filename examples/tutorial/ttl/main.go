// Command ttl is step 2 of the tutorial: after its TTL a result is loaded again.
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
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{
		OnResult: func(_ context.Context, o gashe.Outcome) { fmt.Printf("%s: %s\n", o.Key, o.Result) },
	})

	users := gashe.For(c, gashe.Policy[User]{
		TTL:    200 * time.Millisecond, // fresh for 200ms; an hour in a real service
		Jitter: 0.1,                    // each TTL is 10% longer or shorter, so keys do not expire together
		Prefix: "user:",
	})

	name := "Ann" // the row in the database
	loadUser := func(context.Context) (User, error) {
		return User{ID: "42", Name: name}, nil
	}
	get := func() {
		u, err := users.GetOrLoad(ctx, "42", loadUser)
		check(err)
		fmt.Println("  got", u.Name)
	}

	get()         // miss: loaded
	name = "Anna" // UPDATE users SET name = 'Anna' WHERE id = 42
	get()         // hit: still Ann, the result is fresh
	fmt.Println("wait for the TTL")
	time.Sleep(300 * time.Millisecond)
	get() // miss: loaded again, Anna
}

// check stops the program on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
