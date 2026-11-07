package redis_test

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

// Without an Output line it compiles but does not run: it needs a Redis server.
func ExampleNew() { //nolint:testableexamples // needs a Redis server
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer func() { _ = client.Close() }()
	c := gashe.New(gasheredis.New(client), gashe.Options{Prefix: "orders:"})

	user, err := gashe.GetOrLoad(ctx, c, "user:42", gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (string, error) { return "Ann", nil })
	fmt.Println(user, err)
}

// A near cache over Redis whose Bus tells the other processes which keys changed.
// Without an Output line it compiles but does not run: it needs a Redis server.
func ExampleNewBus() { //nolint:testableexamples // needs a Redis server
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer func() { _ = client.Close() }()
	local := memory.New(memory.Options{MaxEntries: 10_000})
	defer local.Close()

	near := nearcache.New(local, gasheredis.New(client), nearcache.Options{
		LocalTTL: time.Minute,
		Bus:      gasheredis.NewBus(client, "orders:changed"),
	})
	defer func() { _ = near.Close() }()
	c := gashe.New(near, gashe.Options{})
	_ = c
}
