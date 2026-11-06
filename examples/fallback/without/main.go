// Command fallback loses its Redis in the middle of the work. Without Policy.Fallback every
// request goes to the database while Redis is down.
package main

import (
	"context"
	"errors"
	"fmt"
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
	redis := &flaky{Backend: mem}
	c := gashe.New(redis, gashe.Options{})

	policy := gashe.Policy[string]{
		TTL:    time.Hour,
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
	fmt.Println("Redis up, 100 requests, database queries:", queries)

	redis.down = true
	queries = 0
	for range 100 {
		gashe.GetOrLoad(ctx, c, "42", policy, loadUser)
	}
	fmt.Println("Redis down, 100 requests, database queries:", queries)
}
