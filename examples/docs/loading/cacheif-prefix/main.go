// Command cacheif-prefix stores only non-empty results, and puts a prefix before the keys.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type (
	Item struct{ Title string }
	User struct{ Name string }
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:example]
	// skip empty results
	feed := gashe.For(c, gashe.Policy[[]Item]{
		TTL:   gashe.TTL{For: time.Minute},
		Store: gashe.Store[[]Item]{If: func(v []Item) bool { return len(v) > 0 }},
	})

	// stores users:v2:42
	users := gashe.For(c, gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "users:v2:",
	})

	feed.GetOrLoad(ctx, "home", func(context.Context) ([]Item, error) { return nil, nil }) // empty: not stored
	users.GetOrLoad(ctx, "42", func(context.Context) (User, error) { return User{Name: "Ann"}, nil })
	// --8<-- [end:example]
}
