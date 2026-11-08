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
		// the most entries; the least recently used goes first; 0 means 1000
		MaxEntries: 10_000,
		// the most bytes of keys and values together; 0 means no limit
		MaxBytes: 64 << 20,
		// how often expired entries are purged; 0 means 1s, negative turns it
		// off
		PurgeInterval: time.Second,
	})
	defer local.Close()
	// in a service: redis.New(client), see Backends
	shared := memory.New(memory.Options{})
	defer shared.Close()

	near := nearcache.New(local, shared, nearcache.Options{
		// how long a value from shared stays in memory; 0 means 5s
		LocalTTL: 5 * time.Second,
		// tells other processes what changed; nil means they wait LocalTTL
		Bus: nil,
	})
	defer near.Close()
	// --8<-- [end:example]
}
