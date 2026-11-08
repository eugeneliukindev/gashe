// Command redis-nearcache: a copy in process memory in front of Redis, dropped through a bus.
package main

import (
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

func main() {
	// --8<-- [start:example]
	client := goredis.NewUniversalClient(&goredis.UniversalOptions{Addrs: []string{"localhost:6379"}})
	defer client.Close()

	local := memory.New(memory.Options{MaxBytes: 64 << 20})
	defer local.Close()

	backend := nearcache.New(local, gasheredis.New(client), nearcache.Options{
		LocalTTL: time.Minute,
		Bus:      gasheredis.NewBus(client, "orders:invalidate"), // one channel per service
	})
	defer backend.Close() // stops listening to the bus

	_ = gashe.New(backend, gashe.Options{})
	// --8<-- [end:example]
}
