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
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.lockPoll): // since Go 1.23 an unfired timer is garbage collected
		}
	}
}

// TryLock takes the lock key for ttl if it is free; the bool is false when someone else holds it.
// The lock is shared by every process that uses the same backend.
func (c *Cache) TryLock(ctx context.Context, key string, ttl time.Duration) (lease *Lease, ok bool, err error) {
	if ttl <= 0 {
		return nil, false, ErrInvalidTTL
	}
	id := make([]byte, ownerBytes)
	if _, err = rand.Read(id); err != nil {
		return nil, false, fmt.Errorf("gashe: lock %q: %w", key, err)
	}
	owner := []byte(hex.EncodeToString(id))
	ok, err = c.backend.SetIfAbsent(ctx, key, owner, ttl)
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

// WithLock runs fn while holding the lock key, waiting for it as Lock does. It extends the lock
// by ttl every third of ttl while fn runs, so fn may take longer than ttl, and releases it when fn
// returns. If the lock cannot be extended, the ctx of fn is cancelled with that error as its
// cause, and WithLock returns it unless fn returned an error of its own.
func (c *Cache) WithLock(ctx context.Context, key string, ttl time.Duration, fn func(ctx context.Context) error) error {
	lease, err := c.Lock(ctx, key, ttl)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lease.Unlock(ctx); unlockErr != nil {
			c.onError(ctx, fmt.Errorf("gashe: unlock %q: %w", key, unlockErr))
		}
	}()
	held, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stop := lease.keepExtended(held, cancel, ttl)
	err = fn(held)
	stop()
	if cause := context.Cause(held); err == nil && cause != nil && ctx.Err() == nil {
		return cause
	}
	return err
}

// extendsPerTTL is how many times WithLock extends a lock within one ttl.
const extendsPerTTL = 3

// keepExtended extends the lock by ttl in the background until stop is called or ctx ends, and
// cancels ctx with the error if an extension fails. stop returns once the extending has ended.
func (l *Lease) keepExtended(ctx context.Context, cancel context.CancelCauseFunc, ttl time.Duration) (stop func()) {
	quit := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		tick := time.NewTicker(max(ttl/extendsPerTTL, time.Millisecond))
		defer tick.Stop()
		for {
			select {
			case <-quit:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := l.Extend(ctx, ttl); err != nil {
					cancel(fmt.Errorf("gashe: extend lock %q: %w", l.key, err))
					return
				}
			}
		}
	}()
	return func() {
		close(quit)
		<-ended
	}
}
