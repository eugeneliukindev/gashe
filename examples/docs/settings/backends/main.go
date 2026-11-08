// Command backends sets every field of memory.Options and nearcache.Options.
package main

import (
	"time"

	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
)

func main() {
	// --8<-- [start:example]
	local := memory.New(memory.Options{
		MaxEntries:    10_000,      // the most entries; the least recently used goes first; 0 means 1000
		MaxBytes:      64 << 20,    // the most bytes of keys and values together; 0 means no limit
		PurgeInterval: time.Second, // how often expired entries are purged; 0 means 1s, negative turns it off
	})
	defer local.Close()
	shared := memory.New(memory.Options{}) // in a service: redis.New(client), see Backends
	defer shared.Close()

	near := nearcache.New(local, shared, nearcache.Options{
		LocalTTL: 5 * time.Second, // how long a value from shared stays in memory; 0 means 5s
		Bus:      nil,             // tells other processes what changed; nil means they wait LocalTTL
	})
	defer near.Close()
	// --8<-- [end:example]
}
