// Command invalidation is step 4 of the tutorial: after a write, a tag drops every cached user at
// once, and Cached.Set stores the new result right away.
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
		TTL:    time.Hour,
		Tags:   []string{"users"}, // every user carries the tag
		Prefix: "user:",
	})

	db := map[string]string{"42": "Ann", "7": "Bob"} // the users table
	get := func(id string) {
		u, err := users.GetOrLoad(ctx, id, func(context.Context) (User, error) {
			return User{ID: id, Name: db[id]}, nil
		})
		check(err)
		fmt.Println("  got", u.Name)
	}

	fmt.Println("read:")
	get("42") // miss: Ann
	get("7")  // miss: Bob

	fmt.Println("rename 42 to Anna, then InvalidateTags:")
	db["42"] = "Anna"                     // UPDATE users SET name = 'Anna' WHERE id = 42
	check(c.InvalidateTags(ctx, "users")) // after the write, not before
	get("42")                             // miss: Anna
	get("7")                              // miss: Bob, the tag dropped it too

	fmt.Println("rename 42 to Annie, then Set:")
	u := User{ID: "42", Name: "Annie"}
	db["42"] = u.Name
	check(users.Set(ctx, "42", u)) // store the new result you already have
	get("42")                      // hit: Annie
}

// check stops the program on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
