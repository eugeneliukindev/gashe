// Command stampedes/max-loads: at most 20 loads of users run at once.
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
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:example]
	userCache := gashe.For(c, gashe.Policy[User]{
		TTL:      gashe.TTL{For: time.Hour},
		Stale:    gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}},
		Stampede: gashe.Stampede{MaxLoads: 20}, // (1)!
	})

	userCache.GetOrLoad(ctx, "42", func(context.Context) (User, error) {
		return User{Name: "Ann"}, nil
	}) // Ann, miss
	// --8<-- [end:example]
}
