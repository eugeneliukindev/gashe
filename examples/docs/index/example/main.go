// Command example loads a user once, then answers from Redis.
package main

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	// --8<-- [start:example]
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()
	c := gashe.New(gasheredis.New(client), gashe.Options{Prefix: "app:"})
	defer c.Shutdown(ctx)

	users := gashe.For(c, gashe.Policy[User]{
		TTL:      gashe.TTL{For: time.Hour},
		Prefix:   "user:",
		Stale:    gashe.Stale{WhileRevalidate: 10 * time.Minute}, // (1)!
		Stampede: gashe.Stampede{LockFor: 5 * time.Second},       // (2)!
	})

	u, err := users.GetOrLoad(ctx, "42", func(ctx context.Context) (User, error) {
		return User{Name: "Ann"}, nil // your database query
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(u.Name)
	// --8<-- [end:example]
}
