// Command fallback loses its Redis in the middle of the work. With Policy.Fallback the database
// gets one load per key while Redis is down.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// flaky stands for Redis: once down, every read fails, as with a lost connection.
type flaky struct {
	gashe.Backend

	down bool
}

func (f *flaky) Get(ctx context.Context, key string) ([]byte, error) {
	if f.down {
		return nil, errors.New("redis: connection refused")
	}
	return f.Backend.Get(ctx, key)
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	// --8<-- [start:api]
	redis := &flaky{Backend: mem} // Redis, which fails once down is set
	c := gashe.New(redis, gashe.Options{})

	spare := memory.New(memory.Options{MaxBytes: 32 << 20}) // process memory
	defer spare.Close()
	policy := gashe.Policy[string]{
		TTL: gashe.TTL{For: time.Hour},
		Fallback: gashe.Fallback{
			Backend: spare,
			MaxTTL:  30 * time.Second,
		}, // answer from spare while Redis fails
		Prefix: "user:",
	}

	queries := 0
	loadUser := func(context.Context) (string, error) {
		queries++ // a query to the database
		return "Ann", nil
	}

	for range 100 {
		gashe.GetOrLoad(ctx, c, "42", policy, loadUser)
	}
	// Redis up: queries is 1

	redis.down = true
	queries = 0
	for range 100 {
		gashe.GetOrLoad(ctx, c, "42", policy, loadUser)
	}
	// Redis down: queries is 1 again, the result comes from Fallback
	// --8<-- [end:api]
}
