package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backendtest"
	"github.com/eugeneliukindev/gashe/memory"
)

func newMemory(t *testing.T, o memory.Options) *memory.Backend {
	t.Helper()
	if o.CheckInterval == 0 {
		o.CheckInterval = -1
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
	m := newMemory(t, memory.Options{Size: 2})
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

func TestNewPanicsOnNegativeSize(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	memory.New(memory.Options{Size: -1})
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
