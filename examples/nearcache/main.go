// Command nearcache runs two near caches in front of one Redis, standing for two processes: each
// keeps its own local memory, and only Redis is shared. With a Bus, a change made by one process
// drops the local copy of the other at once.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

func main() {
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("this example needs Redis on localhost:6379: %v", err)
	}

	// Process A: its own local memory in front of the shared Redis.
	localA := memory.New(memory.Options{})
	defer localA.Close()
	nearA := nearcache.New(localA, gasheredis.New(client), nearcache.Options{
		LocalTTL: time.Hour, // long on purpose: without the Bus, a change would stay unseen for an hour
		Bus:      gasheredis.NewBus(client, "gashe-example:invalidate"),
	})
	defer nearA.Close()
	a := gashe.New(nearA, gashe.Options{})

	// Process B: the same, with a local memory of its own.
	localB := memory.New(memory.Options{})
	defer localB.Close()
	nearB := nearcache.New(localB, gasheredis.New(client), nearcache.Options{
		LocalTTL: time.Hour,
		Bus:      gasheredis.NewBus(client, "gashe-example:invalidate"),
	})
	defer nearB.Close()
	b := gashe.New(nearB, gashe.Options{})

	time.Sleep(100 * time.Millisecond) // let both processes subscribe

	var price int
	b.Set(ctx, "price", 100, 0)
	a.Get(ctx, "price", &price)
	fmt.Println("process A reads:", price) // process A reads: 100, now kept in its memory

	b.Set(ctx, "price", 120, 0)
	time.Sleep(50 * time.Millisecond) // the message travels through Redis
	a.Get(ctx, "price", &price)
	fmt.Println("process B changed it, process A reads:", price) // process B changed it, process A reads: 120
}
