package nearcache_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe/backend/nearcache"
)

func TestLocalCopyExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		local, shared := newMemory(t), newMemory(t)
		b := nearcache.New(local, shared, nearcache.Options{LocalTTL: 5 * time.Second})

		_ = shared.Set(ctx, "k", []byte("v1"), 0)
		_, _ = b.Get(ctx, "k")
		_ = shared.Set(ctx, "k", []byte("v2"), 0)
		time.Sleep(5 * time.Second)
		if v, _ := b.Get(ctx, "k"); string(v) != "v2" {
			t.Fatalf("Get = %q, want v2 after LocalTTL", v)
		}
	})
}
