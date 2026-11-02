package nearcache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
)

// fakeBus delivers published keys to every subscriber at once, like one pub/sub channel.
type fakeBus struct {
	mu    sync.Mutex
	next  int
	subs  map[int]func([]string)
	ready chan struct{} // one value per Subscribe, once it listens
}

func newBus() *fakeBus {
	return &fakeBus{subs: map[int]func([]string){}, ready: make(chan struct{}, 8)}
}

func (f *fakeBus) Publish(_ context.Context, keys []string) error {
	f.mu.Lock()
	subs := make([]func([]string), 0, len(f.subs))
	for _, s := range f.subs {
		subs = append(subs, s)
	}
	f.mu.Unlock()
	for _, s := range subs {
		s(keys)
	}
	return nil
}

func (f *fakeBus) Subscribe(ctx context.Context, changed func([]string)) error {
	f.mu.Lock()
	id := f.next
	f.next++
	f.subs[id] = changed
	f.mu.Unlock()
	changed(nil)
	f.ready <- struct{}{}
	<-ctx.Done()
	f.mu.Lock()
	delete(f.subs, id)
	f.mu.Unlock()
	return nil
}

func (f *fakeBus) listeners() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

// pod is the near cache of one process over the shared backend.
func pod(t *testing.T, shared gashe.Backend, bus *fakeBus) *nearcache.Backend {
	t.Helper()
	b := nearcache.New(newMemory(t), shared, nearcache.Options{LocalTTL: time.Hour, Bus: bus})
	t.Cleanup(func() { _ = b.Close() })
	<-bus.ready
	return b
}

func TestBusDropsCopiesOfOtherProcesses(t *testing.T) {
	ctx := context.Background()
	shared, bus := newMemory(t), newBus()
	a, b := pod(t, shared, bus), pod(t, shared, bus)

	_ = shared.Set(ctx, "price", []byte("1"), 0)
	if v, _ := a.Get(ctx, "price"); string(v) != "1" { // a keeps a local copy for an hour
		t.Fatalf("a read %q", v)
	}
	_ = b.Set(ctx, "price", []byte("2"), 0)
	if v, _ := a.Get(ctx, "price"); string(v) != "2" {
		t.Fatalf("a still answers %q after b changed it; want the new value at once", v)
	}
}

func TestBusMakesTagInvalidationInstant(t *testing.T) {
	ctx := context.Background()
	shared, bus := newMemory(t), newBus()
	ca, cb := gashe.New(pod(t, shared, bus), gashe.Options{}), gashe.New(pod(t, shared, bus), gashe.Options{})
	users := gashe.Policy[int]{TTL: time.Hour, Tags: []string{"users"}}
	calls, load := versions()
	if err := cb.InvalidateTags(ctx, "users"); err != nil { // the counter exists, so a can copy it
		t.Fatal(err)
	}

	_, _ = gashe.GetOrLoad(ctx, ca, "u", users, load)
	_, _ = gashe.GetOrLoad(ctx, ca, "u", users, load) // a hit, the tag counter is now copied in a
	if err := cb.InvalidateTags(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	if got, _ := gashe.GetOrLoad(ctx, ca, "u", users, load); got != 2 || calls.Load() != 2 {
		t.Fatalf("a answered v%d after b invalidated the tag; want a reload", got)
	}
}

func TestBusResetDropsEverything(t *testing.T) {
	ctx := context.Background()
	shared, bus := newMemory(t), newBus()
	a := pod(t, shared, bus)

	_ = shared.Set(ctx, "k", []byte("1"), 0)
	_, _ = a.Get(ctx, "k")
	_ = shared.Set(ctx, "k", []byte("2"), 0) // changed behind the bus, as while it was disconnected
	_ = bus.Publish(ctx, nil)                // what the bus sends after a reconnect
	if v, _ := a.Get(ctx, "k"); string(v) != "2" {
		t.Fatalf("a read %q after a reset; want the shared value", v)
	}
}

func TestCloseStopsListening(t *testing.T) {
	bus := newBus()
	b := nearcache.New(newMemory(t), newMemory(t), nearcache.Options{Bus: bus})
	<-bus.ready
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if n := bus.listeners(); n != 0 {
		t.Fatalf("%d listeners after Close", n)
	}
	if err := b.Close(); err != nil {
		t.Fatal("second Close:", err)
	}
}

// versions returns a loader that answers 1, 2, ... and counts its runs.
func versions() (*atomic.Int32, func(context.Context) (int, error)) {
	var n atomic.Int32
	return &n, func(context.Context) (int, error) { return int(n.Add(1)), nil }
}
