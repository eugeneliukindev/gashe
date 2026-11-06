// Command fallback loses its Redis in the middle of the work: without a fallback every request
// goes to the database, with Policy.Fallback the database gets one load per key.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// flaky stands for Redis: down makes every call fail, as a lost connection does.
type flaky struct {
	gashe.Backend

	down bool
}

var errDown = errors.New("redis: connection refused")

func (f *flaky) Get(ctx context.Context, key string) ([]byte, error) {
	if f.down {
		return nil, errDown
	}
	return f.Backend.Get(ctx, key)
}

func (f *flaky) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if f.down {
		return errDown
	}
	return f.Backend.Set(ctx, key, value, ttl)
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	spare := memory.New(memory.Options{MaxBytes: 32 << 20})
	defer spare.Close()
	redis := &flaky{Backend: mem}
	c := gashe.New(redis, gashe.Options{})

	queries := 0
	loadUser := func(context.Context) (string, error) {
		queries++ // a query to the database
		return "Ann", nil
	}
	plain := gashe.For(c, gashe.Policy[string]{TTL: time.Hour, Prefix: "plain:"})
	kept := gashe.For(c, gashe.Policy[string]{
		TTL:      time.Hour,
		Prefix:   "kept:",
		Fallback: &gashe.FallbackOptions{Backend: spare, MaxTTL: 30 * time.Second},
	})

	run := func(name string, users gashe.Cached[string]) {
		queries = 0
		for range 100 {
			_, _ = users.GetOrLoad(ctx, "42", loadUser)
		}
		fmt.Printf("%-34s 100 requests, %3d database queries\n", name, queries)
	}

	run("Redis up:", plain) // 100 requests,   1 database queries
	redis.down = true
	run("Redis down, no fallback:", plain)         // 100 requests, 100 database queries
	run("Redis down, with Policy.Fallback:", kept) // 100 requests,   1 database queries
}
