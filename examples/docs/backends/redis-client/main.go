// Command redis-client: a backend in Redis, shared by every process.
package main

import (
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

func main() {
	// --8<-- [start:example]
	client := goredis.NewUniversalClient(&goredis.UniversalOptions{
		Addrs:        []string{"localhost:6379"},
		ReadTimeout:  100 * time.Millisecond, // (1)!
		WriteTimeout: 100 * time.Millisecond,
	})
	defer client.Close()
	backend := gasheredis.New(client)

	_ = gashe.New(backend, gashe.Options{})
	// --8<-- [end:example]
}
