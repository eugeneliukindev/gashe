// Command negative asks three times for a user that does not exist. Without Negative the "not
// found" is not cached, so every lookup reaches the database.
package main

import (
	"context"
	"database/sql"
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
	}
	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++
		return User{}, sql.ErrNoRows // a scraper asks for ids that do not exist
	}

	gashe.GetOrLoad(ctx, c, "user:999", policy, loadUser) // sql.ErrNoRows: loaded
	gashe.GetOrLoad(ctx, c, "user:999", policy, loadUser) // sql.ErrNoRows: loaded again
	gashe.GetOrLoad(ctx, c, "user:999", policy, loadUser) // sql.ErrNoRows: loaded again
	// queries is 3
	// --8<-- [end:api]
}
