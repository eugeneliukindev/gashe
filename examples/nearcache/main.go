// Command nearcache runs two "pods" with a local memory cache in front of one Redis. With a Bus,
// a change made by one pod drops the local copy of the other at once.
//
// It needs Redis: docker run --rm -p 6379:6379 redis:7, or set REDIS_ADDR.
package main

import (
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

func main() {
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: cmp.Or(os.Getenv("REDIS_ADDR"), "localhost:6379")})
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("this example needs Redis (docker run --rm -p 6379:6379 redis:7): %v", err)
	}
	defer func() { _ = client.Close() }()

	pod := func() (*gashe.Cache, func()) {
		local := memory.New(memory.Options{})
		near := nearcache.New(local, gasheredis.New(client), nearcache.Options{
			LocalTTL: time.Hour, // long on purpose: without the Bus, a change would stay unseen for an hour
			Bus:      gasheredis.NewBus(client, "gashe-example:invalidate"),
		})
		return gashe.New(near, gashe.Options{}), func() { _ = near.Close(); _ = local.Close() }
	}
	a, closeA := pod()
	defer closeA()
	b, closeB := pod()
	defer closeB()
	time.Sleep(100 * time.Millisecond) // let both pods subscribe

	check(b.Set(ctx, "price", 100, 0))
	fmt.Println("pod A reads:", read(ctx, a)) // pod A reads: 100, now kept in its memory
	check(b.Set(ctx, "price", 120, 0))
	time.Sleep(50 * time.Millisecond)                           // the message travels through Redis
	fmt.Println("pod B changed it, pod A reads:", read(ctx, a)) // pod B changed it, pod A reads: 120
}

func read(ctx context.Context, c *gashe.Cache) int {
	var price int
	check(c.Get(ctx, "price", &price))
	return price
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
