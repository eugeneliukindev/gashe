// Command stampedes/policy: the three fields of Stampede in one policy.
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

	users := gashe.For(c, gashe.Policy[User]{
		TTL: gashe.TTL{For: time.Hour},
		Stampede: gashe.Stampede{
			SingleFlight: true,
			LockFor:      5 * time.Second,
			MaxLoads:     20,
		},
	})

	users.GetOrLoad(ctx, "42", func(context.Context) (User, error) {
		return User{Name: "Ann"}, nil
	}) // Ann, miss
}
