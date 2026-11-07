// Command set stores the result just written to the database, as a load would.
package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

// database stands for *sql.DB.
type database struct{}

func (database) ExecContext(context.Context, string, ...any) (sql.Result, error) { return nil, nil }

func main() {
	if err := run(context.Background()); err != nil {
		panic(err)
	}
}

func run(ctx context.Context) error {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	var db database
	userCache := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "users:v2:"})
	u := User{Name: "Ann"}

	_, err := db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = 42", u.Name)
	if err != nil {
		return err
	}
	return userCache.Set(ctx, "42", u) // (1)!
}
