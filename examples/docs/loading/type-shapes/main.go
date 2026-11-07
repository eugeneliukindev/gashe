// Command type-shapes stores a user, then reads it with a renamed field: the old entry is a miss.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	{
		type User struct{ Name string } // before the deploy
		users := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "user:"})
		users.GetOrLoad(ctx, "42", func(context.Context) (User, error) { return User{Name: "Ann"}, nil }) // miss: stored
	}
	{
		type User struct{ FullName string } // after it: old entries are misses
		users := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "user:"})
		users.GetOrLoad(ctx, "42", func(context.Context) (User, error) { return User{FullName: "Ann Lee"}, nil }) // miss: loads again
	}
}
