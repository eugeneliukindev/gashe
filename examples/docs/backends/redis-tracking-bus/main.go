// Command redis-tracking-bus: Redis itself reports every change of a key under the prefix.
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
	local := memory.New(memory.Options{MaxBytes: 64 << 20})
	defer local.Close()

	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"}) // (1)!
	defer client.Close()

	backend := nearcache.New(local, gasheredis.New(client), nearcache.Options{
		LocalTTL: time.Minute,
		Bus:      gasheredis.NewTrackingBus(client, "orders:"), // (2)!
	})
	defer backend.Close()

	_ = gashe.New(backend, gashe.Options{})
	// --8<-- [end:example]
}
