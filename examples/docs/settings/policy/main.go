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
			// How long a result is fresh: a Duration, "1h", or func(User)
			// time.Duration.
			// e.g. a profile in Postgres changes rarely: an hour
			For: time.Hour,
			// Spreads each TTL by up to this share, 0 to 1; 0 means none.
			// e.g. 10 000 products warmed up at once do not all expire in the
			// same second
			Jitter: 0.1,
		},
		// In front of the keys of this policy; "" means none.
		// e.g. user:42 and order:42 do not collide; users:v2: after a schema
		// change
		Prefix: "user:",
		// What a result past TTL is still good for; the zero value means
		// nothing.
		Stale: gashe.Stale{
			// Answer it and reload in the background; 0 means off.
			// e.g. a top-products query takes 300ms: the last list answers
			// while it runs
			WhileRevalidate: 10 * time.Minute,
			// Answer it while the loader fails.
			// e.g. the Postgres replica is down 5 minutes: last hour's profile,
			// not a 500
			IfError: gashe.StaleIfError{
				For: time.Hour, // how long after TTL; 0 means off
				// Which errors; nil means any.
				// e.g. sql.ErrNoRows is no failure: a deleted user must not
				// show up
				IsFailure: func(err error) bool {
					return !errors.Is(err, errNoUser)
				},
			},
			// Answer it while the loader is slow.
			// e.g. a rates API sometimes takes 10s: the last rate after 1s
			IfSlow: gashe.StaleIfSlow{
				For: time.Hour, // how long after TTL; 0 means off
				// How long a call waits for the loader; set with For.
				After: time.Second,
			},
		},
		// How calls share loads; the zero value shares nothing.
		Stampede: gashe.Stampede{
			// One load per key in this process; false means one per call.
			// e.g. 200 requests of one product in a process make one SELECT
			SingleFlight: true,
			// One load per key for all processes; 0 means no lock.
			// e.g. 20 pods restarted at once read a cold key: one SELECT for
			// all of them
			LockFor: 5 * time.Second,
			// Loads of this kind at once in a process; 0 means no limit.
			// e.g. 5000 cold reports after a flush: at most 100 queries hold
			// the pool
			MaxLoads: 100,
		},
		// InvalidateTags of any of them drops the result; nil means none.
		// e.g. InvalidateTags(ctx, "users") after a migration drops every user
		// at once
		Tags: []string{"users"},
		// Caches "not found" too; the zero value turns it off.
		// e.g. bots try ids that do not exist: one SELECT per id and minute
		Negative: gashe.Negative{
			For: time.Minute, // how long the "not found" is kept
			// Which loader errors mean "not found".
			IsNotFound: func(err error) bool {
				return errors.Is(err, errNoUser)
			},
		},
		// Answers from a spare backend while the main one fails.
		// e.g. the cache backend is down: memory answers, one load per key
		Fallback: gashe.Fallback{
			// The spare, usually memory; nil means no fallback.
			Backend: spare,
			// The longest a result stays there; 0 means 30s.
			MaxTTL: 30 * time.Second,
		},
		// Which results are stored; the zero value stores every one.
		Store: gashe.Store[User]{
			// Only loads that took this long; 0 means every load.
			// e.g. an index lookup of 2ms is not cached, a full-text search of
			// 800ms is
			SlowerThan: 50 * time.Millisecond,
			// Only results it accepts; nil means all.
			// e.g. an empty result is not kept: the row may appear a second
			// later
			If: func(u User) bool { return u.Name != "" },
		},
	})

	u, err := users.GetOrLoad(ctx, "42", func(context.Context) (User, error) {
		return User{Name: "Ann"}, nil // SELECT name FROM users WHERE id = 42
	})
	fmt.Println(u.Name, err)
	// --8<-- [end:example]
}
