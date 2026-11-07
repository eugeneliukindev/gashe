// Command staleif shows what Stale.IfError.IsFailure decides: which loader errors are failures the
// stale result answers. A database that is down gets the stale result; a deleted row gets its error.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:api]
	errDown := errors.New("database is down")
	errDeleted := errors.New("page is deleted")

	policy := gashe.Policy[string]{
		TTL: gashe.TTL{For: time.Second},
		Stale: gashe.Stale{
			IfError: gashe.StaleIfError{
				For:       time.Minute,
				IsFailure: func(err error) bool { return !errors.Is(err, errDeleted) },
			},
		},
	}

	v1 := func(context.Context) (string, error) { return "v1", nil }
	down := func(context.Context) (string, error) { return "", errDown }
	deleted := func(context.Context) (string, error) { return "", errDeleted }

	gashe.GetOrLoad(ctx, c, "page", policy, v1) // "v1": stored

	time.Sleep(1100 * time.Millisecond)              // past the TTL
	gashe.GetOrLoad(ctx, c, "page", policy, down)    // "v1": the stale result
	gashe.GetOrLoad(ctx, c, "page", policy, deleted) // "", page is deleted
	// --8<-- [end:api]
}
