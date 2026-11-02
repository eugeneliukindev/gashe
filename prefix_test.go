package gashe_test

import (
	"context"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backendtest"
)

func TestWithPrefixContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		t.Helper()
		return gashe.WithPrefix(newCache(t).Backend(), "svc*[1]:")
	})
}

func TestWithPrefixKeepsServicesApart(t *testing.T) {
	ctx := context.Background()
	shared := memory.New(memory.Options{PurgeInterval: -1})
	defer shared.Close()
	orders := gashe.WithPrefix(shared, "orders:")
	users := gashe.WithPrefix(shared, "users:")

	_ = orders.Set(ctx, "1", []byte("o"), 0)
	_ = users.Set(ctx, "1", []byte("u"), 0)
	if v, _ := orders.Get(ctx, "1"); string(v) != "o" {
		t.Fatalf("orders read %q", v)
	}
	_ = users.DeleteMatch(ctx, "*")
	if v, err := shared.Get(ctx, "orders:1"); err != nil || string(v) != "o" {
		t.Fatalf("DeleteMatch of one service removed another's key: %q, %v", v, err)
	}
}
