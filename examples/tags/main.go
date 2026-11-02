// Command tags invalidates everything cached about a user with one call after a write, and
// shows that the next read loads fresh data.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Profile struct{ Name string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	db := map[string]string{"42": "Ann"}
	read := func(id string) {
		p, err := gashe.GetOrLoad(ctx, c, "profile:"+id, gashe.Policy[Profile]{
			TTL:  time.Hour,
			Tags: []string{"user:" + id}, // one tag per user
		}, func(context.Context) (Profile, error) {
			fmt.Println("  load from the database")
			return Profile{Name: db[id]}, nil
		})
		check(err)
		fmt.Println("  profile:", p.Name)
	}

	fmt.Println("read twice:")
	read("42") // load from the database, profile: Ann
	read("42") // profile: Ann, from the cache

	fmt.Println("rename the user, then invalidate the tag:")
	db["42"] = "Anna" // UPDATE users SET name = 'Anna' WHERE id = 42
	check(c.InvalidateTags(ctx, "user:42"))
	read("42") // load from the database, profile: Anna
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
