// Command cacheaside caches a slow database call with GetOrLoad: the first call loads, the next
// ones are answered from the cache until TTL ends.
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
	ID   int
	Name string
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	queries := 0
	loadUser := func(_ context.Context, id int) (User, error) {
		queries++
		time.Sleep(100 * time.Millisecond) // a slow query
		return User{ID: id, Name: "Ann"}, nil
	}

	for range 3 {
		start := time.Now()
		u, err := gashe.GetOrLoad(ctx, c, "42", gashe.Policy[User]{
			TTL:    time.Minute,
			Prefix: "user:",
		}, func(ctx context.Context) (User, error) {
			return loadUser(ctx, 42)
		})
		check(err)
		// {ID:42 Name:Ann} in 100ms, then {ID:42 Name:Ann} in 0s twice: the cache answers
		fmt.Printf("%+v in %v\n", u, time.Since(start).Round(time.Millisecond))
	}
	fmt.Println("database queries:", queries) // database queries: 1
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
