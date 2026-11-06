// Command negative asks three times for a user that does not exist. With Negative the "not
// found" is cached, so the database gets one query.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

	policy := gashe.Policy[User]{
		TTL: time.Hour,
		Negative: gashe.Negative{ // cache "not found" for 30 seconds
			If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) }, // which errors mean "not found"
			Err: sql.ErrNoRows,                                                 // what a cached "not found" returns
			TTL: 30 * time.Second,
		},
	}
	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++
		return User{}, sql.ErrNoRows // a scraper asks for ids that do not exist
	}

	for range 3 {
		_, err := gashe.GetOrLoad(ctx, c, "user:999", policy, loadUser)
		fmt.Println("err:", err)
	}
	fmt.Println("database queries:", queries)
}
