package redis_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

var _ nearcache.Bus = (*gasheredis.Bus)(nil)

// eventually polls cond for up to five seconds: pub/sub delivers messages asynchronously.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal(what)
}

func TestBusOverRedis(t *testing.T) {
	ctx := context.Background()
	client := newClient(t)
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	newPod := func() *nearcache.Backend {
		local := memory.New(memory.Options{PurgeInterval: -1})
		t.Cleanup(func() { _ = local.Close() })
		b := nearcache.New(local, gasheredis.New(client), nearcache.Options{
			LocalTTL: time.Hour,
			Bus:      gasheredis.NewBus(newClient(t), "gashe:test:invalidate"),
		})
		t.Cleanup(func() { _ = b.Close() })
		return b
	}
	a, b := newPod(), newPod()
	read := func(want string) func() bool {
		return func() bool { v, err := a.Get(ctx, "price"); return err == nil && string(v) == want }
	}

	if err := b.Set(ctx, "price", []byte("1"), 0); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a never read 1", read("1")) // a now keeps a local copy for an hour
	if err := b.Set(ctx, "price", []byte("2"), 0); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a kept its copy after b changed the key", read("2"))

	// A change behind the bus, then a lost subscription: after it reconnects, a drops everything.
	if err := client.Set(ctx, "price", "3", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, "CLIENT", "KILL", "TYPE", "pubsub").Err(); err != nil {
		t.Skipf("the server cannot drop pub/sub clients (%v), so the reconnect is not checked", err)
	}
	eventually(t, "a kept its copy after the bus reconnected", read("3"))
}

var _ nearcache.Bus = (*gasheredis.TrackingBus)(nil)

func TestTrackingBusSeesChangesBehindGashe(t *testing.T) {
	ctx := context.Background()
	client := newClient(t)
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	local := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = local.Close() })
	pod := nearcache.New(local, gasheredis.New(client), nearcache.Options{
		LocalTTL: time.Hour,
		Bus:      gasheredis.NewTrackingBus(newClient(t), "t:"),
	})
	t.Cleanup(func() { _ = pod.Close() })
	read := func(want string) func() bool {
		return func() bool { v, err := pod.Get(ctx, "t:price"); return err == nil && string(v) == want }
	}
	set := func(v string) {
		t.Helper()
		if err := client.Set(ctx, "t:price", v, 0).Err(); err != nil { // as redis-cli would, past gashe
			t.Fatal(err)
		}
	}

	set("1")
	eventually(t, "the pod never read 1", read("1")) // the pod now keeps a local copy for an hour
	set("2")
	eventually(t, "the pod kept its copy after a change behind gashe", read("2"))

	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the pod kept its copy after FLUSHDB", func() bool {
		_, err := pod.Get(ctx, "t:price")
		return errors.Is(err, gashe.ErrNotFound)
	})

	// A lost connection: after it reconnects, the pod drops everything and tracks again.
	set("3")
	eventually(t, "the pod never read 3", read("3"))
	if err := client.Do(ctx, "CLIENT", "KILL", "TYPE", "pubsub").Err(); err != nil {
		t.Skipf("the server cannot drop pub/sub clients (%v), so the reconnect is not checked", err)
	}
	set("4")
	eventually(t, "the pod kept its copy after the bus reconnected", read("4"))
	set("5")
	eventually(t, "the pod stopped tracking after the reconnect", read("5"))
}
