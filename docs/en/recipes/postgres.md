# Cache a database query { #cache-a-database-query }

Cache users read from Postgres in Redis, and keep the cache right after an `UPDATE`.

## The code { #the-code }

```bash
go get github.com/eugeneliukindev/gashe github.com/eugeneliukindev/gashe/backend/redis
go get github.com/eugeneliukindev/gashe/codec/msgpack github.com/eugeneliukindev/gashe/compress/zstd
go get github.com/jackc/pgx/v5 github.com/redis/go-redis/v9
```

```go
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // the Postgres driver for database/sql
	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
	"github.com/eugeneliukindev/gashe/codec/msgpack"
	"github.com/eugeneliukindev/gashe/compress/zstd"
)

type User struct {
	ID   int64
	Name string
}

// Users reads users from Postgres through the cache.
type Users struct {
	db    *sql.DB
	cache *gashe.Cache
	users gashe.Cached[User]
}

func NewUsers(db *sql.DB, c *gashe.Cache) *Users {
	return &Users{db: db, cache: c, users: gashe.For(c, gashe.Policy[User]{
		TTL:          10 * time.Minute, // fresh for 10 minutes
		Jitter:       0.1,              // keys stored together do not expire together
		StaleIfError: time.Hour,        // Postgres is down: answer the old user for up to an hour
		LockOnMiss:   5 * time.Second,  // a cold key loads once for all pods
		Negative: gashe.Negative{ // a missing id is asked once per 30 seconds, not on every call
			If:  func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
			Err: sql.ErrNoRows,
			TTL: 30 * time.Second,
		},
		Prefix: "user:", // stores user:42
	})}
}

func (s *Users) Get(ctx context.Context, id int64) (User, error) {
	return s.users.GetOrLoad(ctx, strconv.FormatInt(id, 10), func(ctx context.Context) (User, error) {
		u := User{ID: id} // runs on a miss only
		err := s.db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = $1", id).Scan(&u.Name)
		return u, err
	})
}

// Rename writes to Postgres first, then stores the new user: the next Get is a hit with it.
func (s *Users) Rename(ctx context.Context, id int64, name string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE users SET name = $1 WHERE id = $2", name, id); err != nil {
		return err
	}
	return s.users.Set(ctx, strconv.FormatInt(id, 10), User{ID: id, Name: name})
}

func main() {
	ctx := context.Background()
	db, err := sql.Open("pgx", "postgres://localhost:5432/app")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	client := goredis.NewClient(&goredis.Options{
		Addr:        "localhost:6379",
		ReadTimeout: 100 * time.Millisecond, // a slow cache must not slow the service down
	})
	defer client.Close()

	c := gashe.New(gasheredis.New(client), gashe.Options{
		Prefix:     "app:",          // every key of this service: app:user:42
		Codec:      msgpack.Codec{}, // binary: smaller and faster to decode than JSON
		Compressor: zstd.New(),      // shrinks values before they reach Redis
	})
	defer c.Shutdown(ctx)

	users := NewUsers(db, c)
	if err := users.Rename(ctx, 42, "Ann"); err != nil {
		log.Fatal(err)
	}
	u, err := users.Get(ctx, 42)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(u.Name)
}
```

## How it works { #how-it-works }

| Part | What it does |
|---|---|
| `msgpack.Codec{}`, `zstd.New()` | the recommended setup over Redis: smaller values, faster to decode than JSON; [codec](../concepts/setup.md#codec-the-format-of-values), [compressor](../concepts/setup.md#compressor-large-values) |
| `GetOrLoad` | the query runs on a miss only; concurrent calls for one id share it, [`LockOnMiss`](../concepts/stampedes.md#lockonmiss-one-load-for-all-pods) makes it one query for all pods |
| `Rename` | the `UPDATE` first, then [`Cached.Set`](../concepts/invalidation.md#cachedset): every pod reads the new name next; a failed `UPDATE` leaves the cache alone |
| `StaleIfError` | Postgres is down: the last user answers for up to an hour; [stale data](../concepts/stale.md#staleiferror-staleifslow-answer-old-data-while-the-source-fails) |
| `Negative` | `sql.ErrNoRows` is cached for 30 seconds; [not found](../concepts/negative.md#negative-cache-not-found) |
| `defer c.Shutdown` | runs before `client.Close`: defers run in reverse order |

## Invalidate with tags { #invalidate-with-tags }

When one write changes many entries, tag them and bump the tag after the write.

```go
package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/eugeneliukindev/gashe"
)

type Order struct {
	ID    int64
	Total int64 // cents
}

// OrdersOf caches the orders of a user under the tag of the user.
func OrdersOf(ctx context.Context, db *sql.DB, c *gashe.Cache, userID string) ([]Order, error) {
	policy := gashe.Policy[[]Order]{TTL: time.Hour, Prefix: "orders:", Tags: []string{"user:" + userID}}
	return gashe.GetOrLoad(ctx, c, userID, policy, func(ctx context.Context) ([]Order, error) {
		rows, err := db.QueryContext(ctx, "SELECT id, total FROM orders WHERE user_id = $1", userID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var orders []Order
		for rows.Next() {
			var o Order
			if err := rows.Scan(&o.ID, &o.Total); err != nil {
				return nil, err
			}
			orders = append(orders, o)
		}
		return orders, rows.Err()
	})
}

// DeleteUser writes first, then makes every entry tagged with the user a miss, in every pod.
func DeleteUser(ctx context.Context, db *sql.DB, c *gashe.Cache, userID string) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
		return err
	}
	return c.InvalidateTags(ctx, "user:"+userID)
}
```

- The tag differs per user, so the policy is built per call and passed to `gashe.GetOrLoad`.
- `InvalidateTags` is one `INCRBY` per tag, no scan: every entry with the tag is a miss in every pod;
  [tags](../concepts/invalidation.md#tags-invalidate-many-entries-with-one-write).

## Variations { #variations }

| If you need | Do |
|---|---|
| to invalidate without the new row at hand | `users.Reload(ctx, id, load)` to load and store it now, or `users.Delete(ctx, id)`; [`Reload`](../concepts/invalidation.md#reload) |
| `pgxpool` instead of `database/sql` | call `pool.QueryRow(ctx, ...).Scan(...)` in the loader: gashe sees only the function |
| a page of users by ids | one query for the missing ones: [`GetOrLoadMany`](../concepts/lists.md#getorloadmany) with `WHERE id = ANY($1)` |
| to keep answering while Redis is down | [`Policy.Fallback`](../concepts/outages.md#fallback-answer-from-memory-while-the-backend-is-down) |
