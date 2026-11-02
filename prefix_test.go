package gashe_test

import (
	"context"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backendtest"
)

func TestPrefixContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		t.Helper()
		mem := memory.New(memory.Options{PurgeInterval: -1})
		t.Cleanup(func() { _ = mem.Close() })
		return gashe.New(mem, gashe.Options{Prefix: "svc*[1]:"}).Backend()
	})
}

func TestPrefixKeepsServicesApart(t *testing.T) {
	ctx := context.Background()
	shared := memory.New(memory.Options{PurgeInterval: -1})
	defer shared.Close()
	orders := gashe.New(shared, gashe.Options{Prefix: "orders:"})
	users := gashe.New(shared, gashe.Options{Prefix: "users:"})

	_ = orders.Set(ctx, "1", "o", 0)
	_ = users.Set(ctx, "1", "u", 0)
	var v string
	if err := orders.Get(ctx, "1", &v); err != nil || v != "o" {
		t.Fatalf("orders read %q, %v", v, err)
	}
	_ = users.DeleteMatch(ctx, "*")
	if raw, err := shared.Get(ctx, "orders:1"); err != nil || string(raw) != `"o"` {
		t.Fatalf("DeleteMatch of one service removed another's key: %q, %v", raw, err)
	}
}

func TestPrefixesAddUp(t *testing.T) {
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{Prefix: "orders:"})
	cachedUsers := gashe.Policy[string]{TTL: time.Hour, Prefix: "users:v2:", Tags: []string{"users"}}

	_, _ = gashe.GetOrLoad(ctx, c, "42", cachedUsers, func(context.Context) (string, error) { return "Ann", nil })
	for _, key := range []string{"orders:users:v2:42", "orders:_tag:users"} {
		if ok, _ := mem.Exists(ctx, key); !ok {
			t.Fatalf("no %q: Options.Prefix and Policy.Prefix must add up", key)
		}
	}
}
