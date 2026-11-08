// Command fallback: while the backend fails, loads keep their results in a backend of their own.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:example]
	fallback := gashe.Fallback{
		Backend: memory.New(memory.Options{MaxBytes: 32 << 20}), // (1)!
		MaxTTL:  30 * time.Second,
	}
	userCache := gashe.For(c, gashe.Policy[User]{
		TTL:      gashe.TTL{For: time.Hour},
		Fallback: fallback,
	})

	userCache.GetOrLoad(ctx, "42", func(context.Context) (User, error) { return User{Name: "Ann"}, nil })
	// --8<-- [end:example]
}
