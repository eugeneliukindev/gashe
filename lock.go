package gashe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// lockRetry is how long Lock waits between attempts to take a busy lock.
	lockRetry  = 100 * time.Millisecond
	ownerBytes = 16
)

// UnlockFunc releases a lock taken by Lock or TryLock. It frees the lock only if it is still
// ours, and ignores the cancellation of its ctx so that a finished request does not leave the
// lock behind.
type UnlockFunc func(ctx context.Context) error

// Lock takes the lock key for ttl, waiting while someone else holds it, and returns a function
// that releases it. The lock is shared by every process that uses the backend; ttl frees it in
// case its holder dies. It returns ctx.Err() if ctx ends first.
func (c *Cache) Lock(ctx context.Context, key string, ttl time.Duration) (UnlockFunc, error) {
	for {
		unlock, ok, err := c.TryLock(ctx, key, ttl)
		if err != nil || ok {
			return unlock, err
		}
		timer := time.NewTimer(lockRetry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// TryLock takes the lock key for ttl if it is free and returns a function that releases it;
// the bool is false when someone else holds the lock.
func (c *Cache) TryLock(ctx context.Context, key string, ttl time.Duration) (UnlockFunc, bool, error) {
	if ttl <= 0 {
		return nil, false, ErrInvalidTTL
	}
	id := make([]byte, ownerBytes)
	if _, err := rand.Read(id); err != nil {
		return nil, false, fmt.Errorf("gashe: lock %q: %w", key, err)
	}
	owner := []byte(hex.EncodeToString(id))
	ok, err := c.backend.SetIfAbsent(ctx, key, owner, ttl)
	if err != nil || !ok {
		return nil, false, err
	}
	return func(ctx context.Context) error {
		_, err := c.backend.Unlock(context.WithoutCancel(ctx), key, owner)
		return err
	}, true, nil
}

// IsLocked reports whether someone holds the lock key.
func (c *Cache) IsLocked(ctx context.Context, key string) (bool, error) {
	return c.backend.Exists(ctx, key)
}
