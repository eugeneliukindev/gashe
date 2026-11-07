// Command policy stores a "not found" of the loader for 30 seconds.
package main

import (
	"database/sql"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
)

type User struct{ Name string }

func main() {
	_ = gashe.Policy[User]{
		TTL: gashe.TTL{For: time.Hour},
		Negative: gashe.Negative{
			TTL:        30 * time.Second,
			IsNotFound: func(err error) bool { return errors.Is(err, sql.ErrNoRows) },
		},
	}
}
