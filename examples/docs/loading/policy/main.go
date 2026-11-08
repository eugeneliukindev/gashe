// Command policy: one policy for one kind of data, set once.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	// --8<-- [start:example]
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})

	users := gashe.For(c, gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Hour, Jitter: 0.1},
		Prefix: "user:",
		Stale: gashe.Stale{
			WhileRevalidate: 10 * time.Minute,
		},
		Stampede: gashe.Stampede{SingleFlight: true},
		Tags:     []string{"users"},
	})

	users.GetOrLoad(ctx, "42", func(context.Context) (User, error) { return User{Name: "Ann"}, nil }) // Ann, miss
	// --8<-- [end:example]
}
