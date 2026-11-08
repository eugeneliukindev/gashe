// Command policy shows a policy to start with.
package main

import (
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:example]
	users := gashe.For(c, gashe.Policy[User]{
		TTL:      gashe.TTL{For: time.Hour},                      // (1)!
		Prefix:   "user:",                                        // (2)!
		Stale:    gashe.Stale{WhileRevalidate: 10 * time.Minute}, // (3)!
		Stampede: gashe.Stampede{LockFor: 5 * time.Second},       // (4)!
	})
	_ = users
	// --8<-- [end:example]
}
