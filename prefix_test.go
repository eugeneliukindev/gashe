package gashe_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backendtest"
	"github.com/eugeneliukindev/gashe/memory"
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

func TestGzip(t *testing.T) {
	g := gashe.Gzip{}
	in := user{Name: strings.Repeat("Ann", 1000)}
	raw, err := g.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := gashe.JSON{}.Marshal(in)
	if len(raw) >= len(plain) || bytes.Contains(raw, []byte("AnnAnn")) {
		t.Fatalf("not compressed: %d bytes vs %d", len(raw), len(plain))
	}
	var out user
	if err := g.Unmarshal(raw, &out); err != nil || out != in {
		t.Fatalf("round trip: %v", err)
	}
	if err := g.Unmarshal([]byte("not gzip"), &out); err == nil {
		t.Fatal("garbage decoded")
	}
}
