package main

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib" // the Postgres driver for database/sql
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

func main() {
	ctx := context.Background()
	// --8<-- [start:example]
	db, _ := sql.Open("pgx", "postgres://localhost:5432/app") // connects on the first query
	defer db.Close()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()
	c := gashe.New(gasheredis.New(client), gashe.Options{})
	defer c.Shutdown(ctx)

	_, _ = OrderCount(ctx, db, c, "42") // a miss: the query runs, the entry gets the tag user:42
	_, _ = OrderCount(ctx, db, c, "42") // a hit
	_ = DeleteUser(ctx, db, c, "42")    // every entry tagged user:42 is a miss now
	// --8<-- [end:example]
}
