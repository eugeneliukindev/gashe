package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

// lockDown fails every SetIfAbsent, like an unreachable Redis while taking a lock.
type lockDown struct {
	gashe.Backend
}

func (lockDown) SetIfAbsent(context.Context, string, []byte, time.Duration) (bool, error) {
	return false, errBackendDown
}

func TestTryLock(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)

	lease, ok, err := c.TryLock(ctx, "job", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first TryLock: %v, %v", ok, err)
	}
	if _, ok, _ := c.TryLock(ctx, "job", time.Minute); ok {
		t.Fatal("second TryLock took a held lock")
	}
	if locked, _ := c.IsLocked(ctx, "job"); !locked {
		t.Fatal("IsLocked = false while held")
	}
	if err := lease.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
	if locked, _ := c.IsLocked(ctx, "job"); locked {
		t.Fatal("IsLocked = true after unlock")
	}
	if _, _, err := c.TryLock(ctx, "job", 0); !errors.Is(err, gashe.ErrInvalidTTL) {
		t.Fatalf("TryLock with ttl 0: %v, want ErrInvalidTTL", err)
	}
}

func TestUnlockKeepsSomeoneElsesLock(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	lease, _, _ := c.TryLock(ctx, "job", time.Minute)
	_, _ = c.Delete(ctx, "job") // our lock is gone, as if it expired
	if _, ok, _ := c.TryLock(ctx, "job", time.Minute); !ok {
		t.Fatal("could not take the free lock")
	}
	_ = lease.Unlock(ctx) // the old holder must not release the new lock
	if locked, _ := c.IsLocked(ctx, "job"); !locked {
		t.Fatal("old unlock released someone else's lock")
	}
}

func TestLockReportsBackendFailure(t *testing.T) {
	c := gashe.New(lockDown{Backend: newCache(t).Backend()}, gashe.Options{})
	if _, err := c.Lock(context.Background(), "job", time.Minute); !errors.Is(err, errBackendDown) {
		t.Fatalf("got %v, want the backend error", err)
	}
}

func TestLeaseExtend(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	lease, _, _ := c.TryLock(ctx, "job", time.Minute)
	if err := lease.Extend(ctx, time.Hour); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := c.TTL(ctx, "job"); ttl <= time.Minute {
		t.Fatalf("TTL after Extend = %v, want about an hour", ttl)
	}
	_, _ = c.Delete(ctx, "job") // the lock expired
	if err := lease.Extend(ctx, time.Hour); !errors.Is(err, gashe.ErrLockLost) {
		t.Fatalf("Extend of a lost lock = %v, want ErrLockLost", err)
	}
}
