package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/eugeneliukindev/gashe"
)

// OrderCount caches the number of orders of a user under the tag of the user.
func OrderCount(ctx context.Context, db *sql.DB, c *gashe.Cache, userID string) (int, error) {
	policy := gashe.Policy[int]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "orders:",
		Tags:   []string{"user:" + userID},
	}
	return gashe.GetOrLoad(ctx, c, userID, policy, func(ctx context.Context) (int, error) {
		n := 0
		err := db.QueryRowContext(ctx, "SELECT count(*) FROM orders WHERE user_id = $1", userID).Scan(&n)
		return n, err
	})
}

// DeleteUser writes first, then makes every entry tagged with the user a miss, in every process.
func DeleteUser(ctx context.Context, db *sql.DB, c *gashe.Cache, userID string) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
		return err
	}
	return c.InvalidateTags(ctx, "user:"+userID)
}
