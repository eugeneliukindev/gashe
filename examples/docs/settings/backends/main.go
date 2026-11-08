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
		// The most entries; the least recently used goes first; 0 means 1000.
		MaxEntries: 10_000,
		// The most bytes of keys and values together; 0 means no limit.
		MaxBytes: 64 << 20,
		// How often expired entries are purged; 0 means 1s, negative turns it
		// off.
		PurgeInterval: time.Second,
	})
	defer local.Close()
	// In a service: redis.New(client), see Backends.
	shared := memory.New(memory.Options{})
	defer shared.Close()

	near := nearcache.New(local, shared, nearcache.Options{
		// How long a value from shared stays in memory; 0 means 5s.
		LocalTTL: 5 * time.Second,
		// Tells other processes what changed; nil means they wait LocalTTL.
		Bus: nil,
	})
	defer near.Close()
	// --8<-- [end:example]
}
