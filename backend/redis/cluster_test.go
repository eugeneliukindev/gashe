package redis_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
	"github.com/eugeneliukindev/gashe/backendtest"
)

// newClusterClient connects to the Redis Cluster in GASHE_REDIS_CLUSTER, a comma separated list of
// nodes; without it the test is skipped.
func newClusterClient(t *testing.T) *goredis.ClusterClient {
	t.Helper()
	addrs := os.Getenv("GASHE_REDIS_CLUSTER")
	if addrs == "" {
		t.Skip("no Redis Cluster: GASHE_REDIS_CLUSTER is not set")
	}
	client := goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: strings.Split(addrs, ",")})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func flushCluster(t *testing.T, client *goredis.ClusterClient) {
	t.Helper()
	err := client.ForEachMaster(context.Background(), func(ctx context.Context, node *goredis.Client) error {
		return node.FlushDB(ctx).Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The contract suite on a Cluster: its keys land on different masters, so calls on several keys,
// scans and DeleteMatch cross nodes.
func TestClusterContract(t *testing.T) {
	client := newClusterClient(t)
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		t.Helper()
		flushCluster(t, client)
		return gasheredis.New(client)
	})
}

// GetOrLoad with tags, locks and a stale reload on a Cluster, where an entry, its tag counters and
// its load lock live on different masters.
func TestClusterGetOrLoad(t *testing.T) {
	ctx := context.Background()
	client := newClusterClient(t)
	flushCluster(t, client)
	c := gashe.New(gasheredis.New(client), gashe.Options{Prefix: "cluster:"})
	loads := 0
	users := gashe.For(c, gashe.Policy[string]{
		TTL:      gashe.TTL{For: time.Hour},
		Stampede: gashe.Stampede{LockFor: 5 * time.Second},
		Tags:     []string{"user:42"},
	})
	load := func(context.Context) (string, error) { loads++; return "Ann", nil }
	for range 3 {
		if v, err := users.GetOrLoad(ctx, "42", load); err != nil || v != "Ann" {
			t.Fatalf("GetOrLoad = %q, %v", v, err)
		}
	}
	if err := c.InvalidateTags(ctx, "user:42"); err != nil {
		t.Fatal(err)
	}
	if _, err := users.GetOrLoad(ctx, "42", load); err != nil {
		t.Fatal(err)
	}
	if loads != 2 {
		t.Fatalf("the loader ran %d times, want 2: once, then once after InvalidateTags", loads)
	}
	keys := map[string]bool{}
	for k, err := range c.Keys(ctx, "*") {
		if err != nil {
			t.Fatal(err)
		}
		keys[k] = true
	}
	if !keys["42"] {
		t.Fatalf("Keys on every master = %v, want the entry 42", keys)
	}
}

// Bus over pub/sub on a Cluster: PUBLISH reaches subscribers on every node.
func TestClusterBus(t *testing.T) {
	ctx := context.Background()
	client := newClusterClient(t)
	flushCluster(t, client)
	newProcess := func() *nearcache.Backend {
		local := memory.New(memory.Options{PurgeInterval: -1})
		t.Cleanup(func() { _ = local.Close() })
		b := nearcache.New(local, gasheredis.New(client), nearcache.Options{
			LocalTTL: time.Hour,
			Bus:      gasheredis.NewBus(newClusterClient(t), "gashe:test:invalidate"),
		})
		t.Cleanup(func() { _ = b.Close() })
		return b
	}
	a, b := newProcess(), newProcess()
	read := func(want string) func() bool {
		return func() bool { v, err := a.Get(ctx, "price"); return err == nil && string(v) == want }
	}
	if err := b.Set(ctx, "price", []byte("1"), 0); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a never read 1", read("1"))
	if err := b.Set(ctx, "price", []byte("2"), 0); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a kept its copy after b changed the key on a Cluster", read("2"))
}
