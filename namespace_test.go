package gashe_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// ownKeys runs every feature that keeps keys of its own and returns the keys, sorted.
func ownKeys(t *testing.T, o gashe.Options) []string {
	t.Helper()
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	c := gashe.New(mem, o)

	if err := c.InvalidateTags(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	_ = c.RateLimit(ctx, "ip", gashe.FixedWindow{Calls: 5, Per: time.Minute})
	_ = c.RateLimit(ctx, "ip", gashe.PerMinute(5))
	var keys []string
	for k, err := range c.Keys(ctx, "*") {
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestNamespace(t *testing.T) {
	for _, tc := range []struct {
		namespace string
		prefix    string
	}{{"", "gashe:"}, {"orders:", "orders:"}} {
		keys := ownKeys(t, gashe.Options{Namespace: tc.namespace})
		want := []string{tc.prefix + "rate:fixed:ip", tc.prefix + "rate:sliding:ip:", tc.prefix + "tag:users"}
		if len(keys) != len(want) {
			t.Fatalf("Namespace %q: keys %v", tc.namespace, keys)
		}
		for i, k := range keys {
			if !strings.HasPrefix(k, want[i]) {
				t.Fatalf("Namespace %q: key %q, want it to start with %q", tc.namespace, k, want[i])
			}
		}
	}
}
