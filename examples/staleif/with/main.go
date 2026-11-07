// Command staleif: Stale.IfError.IsFailure picks the loader errors the stale result answers.
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
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})

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
}
