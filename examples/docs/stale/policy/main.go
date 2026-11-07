// Command stale/policy: the three windows of Stale in one policy.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

var errDown = errors.New("database is down")

func isOutage(err error) bool { return errors.Is(err, errDown) }

func main() {
	ctx := context.Background()
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})

	users := gashe.For(c, gashe.Policy[User]{
		TTL: gashe.TTL{For: time.Minute},
		Stale: gashe.Stale{
			WhileRevalidate: 10 * time.Minute,
			IfError: gashe.StaleIfError{
				For:       6 * time.Hour,
				IsFailure: isOutage,
			},
			IfSlow: gashe.StaleIfSlow{
				For:   time.Hour,
				After: 200 * time.Millisecond,
			},
		},
	})

	users.GetOrLoad(ctx, "42", func(context.Context) (User, error) { return User{Name: "Ann"}, nil }) // Ann, miss
}
