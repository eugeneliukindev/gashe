// Command negative asks three times for a user that does not exist. With Negative the "not
// found" is cached, so the database gets one query.
package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:api]
	policy := gashe.Policy[User]{
		TTL: gashe.TTL{For: time.Hour},
		Negative: gashe.Negative{
			// cache "not found" for 30 seconds
			IsNotFound: func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
			TTL:        30 * time.Second,
		},
	}
	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++
		return User{}, sql.ErrNoRows // a scraper asks for ids that do not exist
	}

	gashe.GetOrLoad(ctx, c, "user:999", policy, loadUser) // gashe.ErrNotFound: loaded, stored
	gashe.GetOrLoad(ctx, c, "user:999", policy, loadUser) // gashe.ErrNotFound: cached
	gashe.GetOrLoad(ctx, c, "user:999", policy, loadUser) // gashe.ErrNotFound: cached
	// queries is 1
	// --8<-- [end:api]
}
