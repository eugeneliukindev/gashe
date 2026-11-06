// Command stale shows how a call past the TTL is answered with TTL only: it waits for the
// loader, and fails with it.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{
		OnResult: func(_ context.Context, o gashe.Outcome) {
			fmt.Printf("  %s: %s\n", o.Key, o.Result) // how each call was answered
		},
	})
	defer c.Shutdown(ctx) // waits for the background loads

	policy := gashe.Policy[string]{
		TTL: time.Second,
	}

	// Each call below passes the loader the database would act as at that moment.
	loadV1 := func(context.Context) (string, error) { return "v1", nil }
	loadSlowV2 := func(context.Context) (string, error) {
		time.Sleep(500 * time.Millisecond)
		return "v2", nil
	}
	loadV2 := func(context.Context) (string, error) { return "v2", nil }
	fail := func(context.Context) (string, error) { return "", errors.New("database is down") }

	fmt.Println("The database has v1:")
	v, _ := gashe.GetOrLoad(ctx, c, "page", policy, loadV1)
	fmt.Println("  got", v)

	fmt.Println("Past the TTL, the database is slow and has v2:")
	time.Sleep(1100 * time.Millisecond)
	v, _ = gashe.GetOrLoad(ctx, c, "page", policy, loadSlowV2)
	fmt.Println("  got", v)
	time.Sleep(600 * time.Millisecond) // a load in the background, if any, is done
	v, _ = gashe.GetOrLoad(ctx, c, "page", policy, loadV2)
	fmt.Println("  got", v)

	fmt.Println("Past the TTL, the database is down:")
	time.Sleep(1100 * time.Millisecond)
	v, err := gashe.GetOrLoad(ctx, c, "page", policy, fail)
	fmt.Printf("  got %q, err: %v\n", v, err)
}
