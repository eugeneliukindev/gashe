package users

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
)

type User struct{ Name string }

// Users is your code: it takes the cache and the query, so a test passes both.
type Users struct {
	cache gashe.Cached[User]
	query func(ctx context.Context, id string) (User, error)
}

func New(c *gashe.Cache, query func(ctx context.Context, id string) (User, error)) *Users {
	users := gashe.For(c, gashe.Policy[User]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "user:",
	})
	return &Users{cache: users, query: query}
}

func (u *Users) Get(ctx context.Context, id string) (User, error) {
	return u.cache.GetOrLoad(ctx, id, func(ctx context.Context) (User, error) { return u.query(ctx, id) })
}
