// Command nearcache runs two near caches, standing for two processes, in front of one Redis.
package main

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

// process is one process: its own local memory in front of the shared Redis.
func process(client *goredis.Client) *gashe.Cache {
	near := nearcache.New(memory.New(memory.Options{}), gasheredis.New(client), nearcache.Options{
		LocalTTL: time.Hour, // without the Bus, a change is unseen for an hour
		Bus:      gasheredis.NewBus(client, "gashe-example:invalidate"),
	})
	return gashe.New(near, gashe.Options{})
}

func main() {
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	a, b := process(client), process(client)
	time.Sleep(100 * time.Millisecond) // let both processes subscribe

	var price int
	b.Set(ctx, "price", 100, 0)
	a.Get(ctx, "price", &price) // 100, now kept in the memory of A

	b.Set(ctx, "price", 120, 0)
	time.Sleep(50 * time.Millisecond) // the message travels through Redis
	a.Get(ctx, "price", &price)       // 120: the Bus dropped the copy of A
}
