package gashe_test

import (
	"context"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestInvalidateTags(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	tagged := gashe.Policy[int]{TTL: time.Hour, Tags: []string{"users"}}
	other := gashe.Policy[int]{TTL: time.Hour, Tags: []string{"orders"}}
	calls, load := versions()

	_, _ = gashe.GetOrLoad(ctx, c, "a", tagged, load)
	_, _ = gashe.GetOrLoad(ctx, c, "b", other, load)
	if err := c.InvalidateTags(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "a", tagged, load); got != 3 {
		t.Fatalf("after InvalidateTags: v%d, want a reload", got)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "b", other, load); got != 2 {
		t.Fatalf("a key of another tag: v%d, want the stored v2", got)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "a", tagged, load); got != 3 || calls.Load() != 3 {
		t.Fatalf("after the reload: v%d, %d calls; want the stored v3", got, calls.Load())
	}
	if got, _ := gashe.Reload(ctx, c, "a", tagged, load); got != 4 {
		t.Fatalf("Reload: v%d", got)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "a", tagged, load); got != 4 {
		t.Fatalf("after Reload: v%d, want the reloaded v4 kept with the current generation", got)
	}
}

func TestEvictedTagCounterDoesNotRevalidateEntries(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.Policy[int]{TTL: time.Hour, Tags: []string{"users"}}
	calls, load := versions()

	_, _ = gashe.GetOrLoad(ctx, c, "u", o, load)
	if err := c.InvalidateTags(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	// The backend evicts the counter, as Redis may under maxmemory-policy allkeys-lru.
	if _, err := c.Backend().Delete(ctx, "gashe:tag:users"); err != nil {
		t.Fatal(err)
	}
	if got, _ := gashe.GetOrLoad(ctx, c, "u", o, load); got != 2 || calls.Load() != 2 {
		t.Fatalf("got v%d after the counter was evicted; the invalidated v1 came back", got)
	}
}
