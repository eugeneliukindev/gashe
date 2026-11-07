// Command lockfor, without Stampede.LockFor: one process of a service that caches in Redis. Run it
// three times at once: the processes share nothing but Redis.
package main

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

type User struct {
	ID   string
	Name string
}

func main() {
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()
	c := gashe.New(gasheredis.New(client), gashe.Options{})

	users := gashe.For(c, gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "user:",
	})

	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++
		time.Sleep(time.Second) // a slow query: the other processes start meanwhile
		return User{ID: "42", Name: "Ann"}, nil
	}

	users.GetOrLoad(ctx, "42", loadUser) // Ann: every process queries
}
