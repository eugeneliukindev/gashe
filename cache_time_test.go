//go:build go1.25

package gashe_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestSetTTLExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)

		_ = c.Set(ctx, "k", 1, time.Hour)
		if ttl, err := c.TTL(ctx, "k"); err != nil || ttl != time.Hour {
			t.Fatalf("TTL = %v %v", ttl, err)
		}
		time.Sleep(time.Hour)
		var n int
		if err := c.Get(ctx, "k", &n); !errors.Is(err, gashe.ErrNotFound) {
			t.Fatalf("after TTL: %v", err)
		}
	})
}

func TestSetWithoutTTLDropsOldTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)

		_ = c.Set(ctx, "k", 1, time.Hour)
		_ = c.Set(ctx, "k", 2, 0)
		time.Sleep(2 * time.Hour)
		var n int
		if err := c.Get(ctx, "k", &n); err != nil || n != 2 {
			t.Fatalf("got %d %v, want 2 with no expiry", n, err)
		}
		if ttl, _ := c.TTL(ctx, "k"); ttl != gashe.NoExpiry {
			t.Fatalf("TTL = %v, want NoExpiry", ttl)
		}
	})
}

func TestIncrByTTLOnlyOnCreate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)

		_, _ = c.IncrBy(ctx, "n", 5, time.Minute)
		time.Sleep(30 * time.Second)
		_, _ = c.IncrBy(ctx, "n", 1, time.Hour) // must not extend the TTL
		time.Sleep(30 * time.Second)
		if ok, _ := c.Exists(ctx, "n"); ok {
			t.Fatal("counter outlived the TTL it was created with")
		}
	})
}
