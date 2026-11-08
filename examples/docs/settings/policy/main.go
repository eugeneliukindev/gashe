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
			For:    time.Hour, // how long a result is fresh: a Duration, "1h", or func(User) time.Duration
			Jitter: 0.1,       // spreads each TTL by up to this share, 0 to 1; 0 means none
		},
		Prefix: "user:", // in front of the keys of this policy; "" means none
		Stale: gashe.Stale{ // what a result past TTL is still good for; the zero value means nothing
			WhileRevalidate: 10 * time.Minute, // answer it and reload in the background; 0 means off
			IfError: gashe.StaleIfError{ // answer it while the loader fails
				For: time.Hour, // how long after TTL; 0 means off
				IsFailure: func(err error) bool { // which errors; nil means any
					return !errors.Is(err, errNoUser)
				},
			},
			IfSlow: gashe.StaleIfSlow{ // answer it while the loader is slow
				For:   time.Hour,   // how long after TTL; 0 means off
				After: time.Second, // how long a call waits for the loader; set with For
			},
		},
		Stampede: gashe.Stampede{ // how calls share loads; the zero value shares nothing
			SingleFlight: true,            // one load per key in this process; false means one per call
			LockFor:      5 * time.Second, // one load per key for all processes; 0 means no lock
			MaxLoads:     100,             // loads of this kind at once in a process; 0 means no limit
		},
		Tags: []string{"users"}, // InvalidateTags of any of them drops the result; nil means none
		Negative: gashe.Negative{ // caches "not found" too; the zero value turns it off
			TTL: time.Minute, // how long the "not found" is kept
			IsNotFound: func(err error) bool { // which loader errors mean "not found"
				return errors.Is(err, errNoUser)
			},
		},
		Fallback: gashe.Fallback{ // answers from a spare backend while the main one fails
			Backend: spare,            // the spare, usually memory; nil means no fallback
			MaxTTL:  30 * time.Second, // the longest a result stays there; 0 means 30s
		},
		Store: gashe.Store[User]{ // which results are stored; the zero value stores every one
			SlowerThan: 50 * time.Millisecond,                     // only loads that took this long; 0 means every load
			If:         func(u User) bool { return u.Name != "" }, // only results it accepts; nil means all
		},
	})

	u, err := users.GetOrLoad(ctx, "42", func(context.Context) (User, error) { return User{Name: "Ann"}, nil })
	fmt.Println(u.Name, err)
	// --8<-- [end:example]
}
