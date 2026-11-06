// Command stale is step 3 of the tutorial: after the TTL the old result answers at once while it
// reloads (StaleWhileRevalidate), and while the database fails (StaleIfError).
package main

import (
	"context"
	"errors"
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
	defer func() { _ = c.Shutdown(ctx) }() // waits for reloads in the background

	users := gashe.For(c, gashe.Policy[User]{
		TTL:                  200 * time.Millisecond,
		StaleWhileRevalidate: time.Minute, // after the TTL: the old result at once, a reload in the background
		Prefix:               "user:",
	})
	profiles := gashe.For(c, gashe.Policy[User]{
		TTL:          200 * time.Millisecond,
		StaleIfError: time.Hour, // after the TTL: the old result instead of a loader error
		Prefix:       "profile:",
	})

	name, down := "Ann", false // the row in the database, and whether the database is down
	loadUser := func(context.Context) (User, error) {
		if down {
			return User{}, errors.New("database is down")
		}
		return User{ID: "42", Name: name}, nil
	}
	get := func(cached gashe.Cached[User]) {
		u, err := cached.GetOrLoad(ctx, "42", loadUser)
		check(err)
		fmt.Println("  got", u.Name)
	}

	fmt.Println("StaleWhileRevalidate:")
	get(users) // miss: Ann
	name = "Anna"
	time.Sleep(300 * time.Millisecond) // past the TTL
	get(users)                         // stale-while-revalidate: Ann at once, Anna loads in the background
	time.Sleep(100 * time.Millisecond) // the reload is done
	get(users)                         // hit: Anna

	fmt.Println("StaleIfError:")
	get(profiles) // miss: Anna
	down = true
	time.Sleep(300 * time.Millisecond) // past the TTL
	get(profiles)                      // stale-if-error: the loader failed, Anna instead of the error
}

// check stops the program on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
