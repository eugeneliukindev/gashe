package main

import (
	"context"
	"strconv"
)

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
