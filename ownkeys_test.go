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
//
//nolint:gocritic // options by value, as gashe.New takes them
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
	for k, err := range mem.Scan(ctx, "*") { // the keys as stored, with the prefix
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestOwnKeys(t *testing.T) {
	for _, prefix := range []string{"", "orders:"} {
		keys := ownKeys(t, gashe.Options{Prefix: prefix})
		want := []string{prefix + "__rate:fixed:ip", prefix + "__rate:sliding:ip:", prefix + "__tag:users"}
		if len(keys) != len(want) {
			t.Fatalf("Prefix %q: keys %v", prefix, keys)
		}
		for i, k := range keys {
			if !strings.HasPrefix(k, want[i]) {
				t.Fatalf("Prefix %q: key %q, want it to start with %q", prefix, k, want[i])
			}
		}
	}
}
