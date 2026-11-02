// Command negative caches "not found", so lookups of a missing id do not reach the database
// every time.
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

	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++
		return User{}, sql.ErrNoRows // a scraper asks for ids that do not exist
	}
	for range 3 {
		_, err := gashe.GetOrLoad(ctx, c, "user:999", gashe.Policy[User]{
			TTL: time.Hour,
			Negative: gashe.Negative{
				If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
				Err: sql.ErrNoRows,
				TTL: 30 * time.Second,
			},
		}, loadUser)
		fmt.Println("err:", err) // err: sql: no rows in result set, three times
	}
	fmt.Println("database queries:", queries) // database queries: 1
}
