package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // the Postgres driver for database/sql
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
	"github.com/eugeneliukindev/gashe/codec/msgpack"
)

func main() {
	ctx := context.Background()
	// --8<-- [start:example]
	db, _ := sql.Open("pgx", "postgres://localhost:5432/app") // connects on the first query
	defer db.Close()
	client := goredis.NewClient(&goredis.Options{
		Addr:        "localhost:6379",
		ReadTimeout: 100 * time.Millisecond, // (1)!
	})
	defer client.Close()
	c := gashe.New(gasheredis.New(client), gashe.Options{
		Prefix: "app:",          // every key of this service: app:user:42
		Codec:  msgpack.Codec{}, // (2)!
	})
	defer c.Shutdown(ctx)

	users := NewUsers(db, c)
	_ = users.Rename(ctx, 42, "Ann")
	u, _ := users.Get(ctx, 42)
	fmt.Println(u.Name) // Ann
	// --8<-- [end:example]
}
