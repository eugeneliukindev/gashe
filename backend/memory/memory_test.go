package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backendtest"
)

func newMemory(t *testing.T, o memory.Options) *memory.Backend {
	t.Helper()
	if o.PurgeInterval == 0 {
		o.PurgeInterval = -1
	}
	m := memory.New(o)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		t.Helper()
		return newMemory(t, memory.Options{})
	})
}

func TestLRUEvictsLeastRecentlyUsed(t *testing.T) {
	ctx := context.Background()
	m := newMemory(t, memory.Options{MaxEntries: 2})
	_ = m.Set(ctx, "a", []byte("1"), 0)
	_ = m.Set(ctx, "b", []byte("1"), 0)
	_, _ = m.Get(ctx, "a") // a is now the most recent
	_ = m.Set(ctx, "c", []byte("1"), 0)

	if _, err := m.Get(ctx, "b"); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("b survived: %v", err)
	}
	for _, k := range []string{"a", "c"} {
		if _, err := m.Get(ctx, k); err != nil {
			t.Fatalf("%s evicted: %v", k, err)
		}
	}
}

func TestMaxBytesEvictsLeastRecentlyUsed(t *testing.T) {
	ctx := context.Background()
	m := newMemory(t, memory.Options{MaxBytes: 10}) // each entry below is 1+4 bytes
	_ = m.Set(ctx, "a", []byte("1111"), 0)
	_ = m.Set(ctx, "b", []byte("2222"), 0)
	_ = m.Set(ctx, "c", []byte("3333"), 0)

	if _, err := m.Get(ctx, "a"); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("a survived: %v", err)
	}
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2", m.Len())
	}
	// Growing b in place must count too: c goes.
	_ = m.Set(ctx, "b", []byte("22222"), 0)
	if _, err := m.Get(ctx, "c"); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("c survived: %v", err)
	}
	if err := m.Set(ctx, "big", []byte("12345678"), 0); !errors.Is(err, memory.ErrTooLarge) {
		t.Fatalf("Set too large = %v, want ErrTooLarge", err)
	}
	if _, err := m.Get(ctx, "b"); err != nil {
		t.Fatalf("b evicted by a rejected value: %v", err)
	}
}

func TestMaxBytesCountsIncrBy(t *testing.T) {
	ctx := context.Background()
	m := newMemory(t, memory.Options{MaxBytes: 6})
	_ = m.Set(ctx, "a", []byte("1"), 0) // 2 bytes
	_, _ = m.IncrBy(ctx, "n", 1, 0)     // 2 bytes
	_, _ = m.IncrBy(ctx, "n", 99999, 0) // n is now "100000": 7 bytes, alone over the limit

	if _, err := m.Get(ctx, "a"); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("a survived: %v", err)
	}
}

func TestNewPanicsOnNegativeSize(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	memory.New(memory.Options{MaxEntries: -1})
}

func TestCloseTwice(t *testing.T) {
	m := memory.New(memory.Options{})
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}
