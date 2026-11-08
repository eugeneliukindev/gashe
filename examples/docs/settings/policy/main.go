// Command policy sets every field of gashe.Policy, nested ones included.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

var errNoUser = errors.New("no such user")

func main() {
	ctx := context.Background()
	backend := memory.New(memory.Options{})
	defer backend.Close()
	spare := memory.New(memory.Options{})
	defer spare.Close()
	c := gashe.New(backend, gashe.Options{})
	defer c.Shutdown(ctx)

	// --8<-- [start:example]
	users := gashe.For(c, gashe.Policy[User]{
		TTL: gashe.TTL{
			// how long a result is fresh: a Duration, "1h", or func(User)
			// time.Duration
			For: time.Hour,
			// spreads each TTL by up to this share, 0 to 1; 0 means none
			Jitter: 0.1,
		},
		Prefix: "user:", // in front of the keys of this policy; "" means none
		// what a result past TTL is still good for; the zero value means
		// nothing
		Stale: gashe.Stale{
			// answer it and reload in the background; 0 means off
			WhileRevalidate: 10 * time.Minute,
			IfError: gashe.StaleIfError{ // answer it while the loader fails
				For: time.Hour, // how long after TTL; 0 means off
				IsFailure: func(err error) bool { // which errors; nil means any
					return !errors.Is(err, errNoUser)
				},
			},
			IfSlow: gashe.StaleIfSlow{ // answer it while the loader is slow
				For: time.Hour, // how long after TTL; 0 means off
				// how long a call waits for the loader; set with For
				After: time.Second,
			},
		},
		// how calls share loads; the zero value shares nothing
		Stampede: gashe.Stampede{
			// one load per key in this process; false means one per call
			SingleFlight: true,
			// one load per key for all processes; 0 means no lock
			LockFor: 5 * time.Second,
			// loads of this kind at once in a process; 0 means no limit
			MaxLoads: 100,
		},
		// InvalidateTags of any of them drops the result; nil means none
		Tags: []string{"users"},
		// caches "not found" too; the zero value turns it off
		Negative: gashe.Negative{
			For: time.Minute, // how long the "not found" is kept
			// which loader errors mean "not found"
			IsNotFound: func(err error) bool {
				return errors.Is(err, errNoUser)
			},
		},
		// answers from a spare backend while the main one fails
		Fallback: gashe.Fallback{
			// the spare, usually memory; nil means no fallback
			Backend: spare,
			// the longest a result stays there; 0 means 30s
			MaxTTL: 30 * time.Second,
		},
		// which results are stored; the zero value stores every one
		Store: gashe.Store[User]{
			// only loads that took this long; 0 means every load
			SlowerThan: 50 * time.Millisecond,
			// only results it accepts; nil means all
			If: func(u User) bool { return u.Name != "" },
		},
	})

	u, err := users.GetOrLoad(ctx, "42", func(context.Context) (User, error) {
		return User{Name: "Ann"}, nil // SELECT name FROM users WHERE id = 42
	})
	fmt.Println(u.Name, err)
	// --8<-- [end:example]
}
