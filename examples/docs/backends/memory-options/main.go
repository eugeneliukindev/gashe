// Command memory-options: a backend in process memory, with its limits.
package main

import (
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	// --8<-- [start:example]
	backend := memory.New(memory.Options{
		MaxEntries:    10_000,      // (1)!
		MaxBytes:      256 << 20,   // (2)!
		PurgeInterval: time.Minute, // (3)!
	})
	defer backend.Close() // stops the background purge

	_ = gashe.New(backend, gashe.Options{})
	// --8<-- [end:example]
}
