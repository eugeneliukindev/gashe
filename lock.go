package gashe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// defaultLockPoll is how often Lock checks a busy lock unless Options.LockPollInterval says
	// otherwise.
	defaultLockPoll = 100 * time.Millisecond
	ownerBytes      = 16
)

// Lease is a lock held by Lock or TryLock.
type Lease struct {
	c     *Cache
	key   string
	owner []byte
}

// Lock takes the lock key for ttl, waiting while someone else holds it and checking it every
// Options.LockPollInterval. The lock is shared by every process that uses the same backend; ttl
// frees it in case its holder dies. It returns ctx.Err() if ctx ends first.
func (c *Cache) Lock(ctx context.Context, key string, ttl time.Duration) (*Lease, error) {
	for {
		lease, ok, err := c.TryLock(ctx, key, ttl)
		if err != nil || ok {
			return lease, err
		}
		timer := time.NewTimer(c.lockPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// TryLock takes the lock key for ttl if it is free; the bool is false when someone else holds it.
// The lock is shared by every process that uses the same backend.
func (c *Cache) TryLock(ctx context.Context, key string, ttl time.Duration) (*Lease, bool, error) {
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
	return &Lease{c: c, key: key, owner: owner}, true, nil
}

// IsLocked reports whether someone holds the lock key.
func (c *Cache) IsLocked(ctx context.Context, key string) (bool, error) {
	return c.backend.Exists(ctx, key)
}

// Unlock releases the lock if it is still ours. It ignores the cancellation of ctx, so that a
// finished request does not leave the lock behind.
func (l *Lease) Unlock(ctx context.Context) error {
	_, err := l.c.backend.Unlock(context.WithoutCancel(ctx), l.key, l.owner)
	return err
}

// Extend gives the lock a new ttl from now, for work that runs longer than planned. It returns
// ErrLockLost if the lock expired and someone else may hold it.
func (l *Lease) Extend(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	ok, err := l.c.backend.ExtendLock(ctx, l.key, l.owner, ttl)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLockLost
	}
	return nil
}
