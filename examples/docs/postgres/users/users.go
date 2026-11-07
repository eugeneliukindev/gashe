package main

import (
	"database/sql"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
)

type User struct {
	ID   int64
	Name string
}

// Users reads users from Postgres through the cache.
type Users struct {
	db    *sql.DB
	users gashe.Cached[User]
}

func NewUsers(db *sql.DB, c *gashe.Cache) *Users {
	return &Users{db: db, users: gashe.For(c, gashe.Policy[User]{
		TTL:      gashe.TTL{For: 10 * time.Minute, Jitter: 0.1},            // fresh for 10 minutes, keys stored together do not expire together
		Stampede: gashe.Stampede{LockFor: 5 * time.Second},                 // a cold key loads once for all processes
		Stale:    gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}}, // (1)!
		Negative: gashe.Negative{
			IsNotFound: func(err error) bool { return errors.Is(err, sql.ErrNoRows) }, // (2)!
			TTL:        30 * time.Second,
		},
		Prefix: "user:", // stores user:42
	})}
}
