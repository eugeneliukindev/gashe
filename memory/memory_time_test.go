//go:build go1.25

package memory_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/memory"
)

func TestBackgroundPurge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		m := newMemory(t, memory.Options{PurgeInterval: time.Second})
		_ = m.Set(ctx, "k", []byte("1"), 500*time.Millisecond)

		time.Sleep(time.Second)
		synctest.Wait()
		if n := m.Len(); n != 0 {
			t.Fatalf("expired entry was not purged: %d keys", n)
		}
	})
}

func TestExpireAndTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		m := newMemory(t, memory.Options{})
		_ = m.Set(ctx, "k", []byte("1"), 0)

		if ok, _ := m.Expire(ctx, "k", time.Minute); !ok {
			t.Fatal("Expire on an existing key")
		}
		time.Sleep(20 * time.Second)
		if ttl, _ := m.TTL(ctx, "k"); ttl != 40*time.Second {
			t.Fatalf("TTL = %v", ttl)
		}
		time.Sleep(40 * time.Second)
		if _, err := m.TTL(ctx, "k"); !errors.Is(err, gashe.ErrNotFound) {
			t.Fatalf("TTL after expiry: %v", err)
		}
		if ok, _ := m.Expire(ctx, "k", time.Minute); ok {
			t.Fatal("Expire on an expired key")
		}
	})
}

func TestAddAfterExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		m := newMemory(t, memory.Options{})
		_, _ = m.SetIfAbsent(ctx, "lock", []byte("a"), time.Second)
		if ok, _ := m.SetIfAbsent(ctx, "lock", []byte("b"), time.Second); ok {
			t.Fatal("Add took a live key")
		}
		time.Sleep(time.Second)
		if ok, _ := m.SetIfAbsent(ctx, "lock", []byte("b"), time.Second); !ok {
			t.Fatal("Add could not take an expired key")
		}
	})
}
