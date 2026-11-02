package nearcache_test

import (
	"context"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	"github.com/eugeneliukindev/gashe/backendtest"
)

func newMemory(t *testing.T) *memory.Backend {
	t.Helper()
	m := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		t.Helper()
		return nearcache.New(newMemory(t), newMemory(t), nearcache.Options{})
	})
}

func TestReadsFromLocalFirst(t *testing.T) {
	ctx := context.Background()
	local, shared := newMemory(t), newMemory(t)
	b := nearcache.New(local, shared, nearcache.Options{LocalTTL: time.Minute})

	_ = shared.Set(ctx, "k", []byte("v1"), 0)
	if v, _ := b.Get(ctx, "k"); string(v) != "v1" {
		t.Fatalf("Get = %q", v)
	}
	_ = shared.Set(ctx, "k", []byte("v2"), 0) // another process changed it
	if v, _ := b.Get(ctx, "k"); string(v) != "v1" {
		t.Fatalf("Get = %q, want the local copy v1 within LocalTTL", v)
	}
	_ = b.Set(ctx, "k", []byte("v3"), 0) // a write through this backend drops the local copy
	if v, _ := b.Get(ctx, "k"); string(v) != "v3" {
		t.Fatalf("Get after Set = %q, want v3", v)
	}
}
