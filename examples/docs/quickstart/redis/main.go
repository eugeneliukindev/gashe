// Command redis runs the cache on Redis, shared by every process.
package main

import (
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

func main() {
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()
	c := gashe.New(gasheredis.New(client), gashe.Options{})
	_ = c
}
