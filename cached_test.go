package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestCachedSetIsReadByGet(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.For(c, gashe.Policy[user]{TTL: time.Hour, Prefix: "users:", Tags: []string{"users"}})
	calls, load := counter(user{Name: "loaded"})

	if err := users.Set(ctx, "42", user{Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if got, err := users.GetOrLoad(ctx, "42", load); err != nil || got.Name != "Ann" || *calls != 0 {
		t.Fatalf("GetOrLoad after Set = %+v, %v, %d loads; want the stored Ann and no load", got, err, *calls)
	}
	if err := c.InvalidateTags(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	if got, _ := users.GetOrLoad(ctx, "42", load); got.Name != "loaded" {
		t.Fatalf("after InvalidateTags = %+v; Set must store the tags of the policy", got)
	}
}

func TestCachedDeleteAppliesPrefix(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.For(c, gashe.Policy[user]{TTL: time.Hour, Prefix: "users:v2:"})
	calls, load := counter(user{Name: "Ann"})

	_, _ = users.GetOrLoad(ctx, "42", load)
	if existed, err := users.Delete(ctx, "42"); err != nil || !existed {
		t.Fatalf("Delete = %v, %v; want the entry under users:v2:42 removed", existed, err)
	}
	_, _ = users.GetOrLoad(ctx, "42", load)
	if *calls != 2 {
		t.Fatalf("loader ran %d times, want 2: the deleted key must load again", *calls)
	}
}

func TestCachedSetDeletesWhatThePolicyDoesNotStore(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.For(c, gashe.Policy[user]{TTL: time.Hour, CacheIf: func(u user) bool { return u.Name != "" }})
	_, load := counter(user{Name: "Ann"})

	_, _ = users.GetOrLoad(ctx, "42", load)
	if err := users.Set(ctx, "42", user{}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := c.Exists(ctx, "42"); ok {
		t.Fatal("an empty user is not stored by CacheIf, so Set must drop the old entry")
	}
}

func TestCachedManyAndReload(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	n := gashe.For(c, gashe.Policy[int]{TTL: time.Hour})
	got, err := n.GetOrLoadMany(ctx, []string{"a", "b"}, func(context.Context, []string) (map[string]int, error) {
		return map[string]int{"a": 1, "b": 2}, nil
	})
	if err != nil || got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("GetOrLoadMany = %v, %v", got, err)
	}
	if v, err := n.Reload(ctx, "a", func(context.Context) (int, error) { return 10, nil }); err != nil || v != 10 {
		t.Fatalf("Reload = %d, %v", v, err)
	}
	if v, _ := n.GetOrLoad(ctx, "a", func(context.Context) (int, error) { return -1, nil }); v != 10 {
		t.Fatalf("GetOrLoad after Reload = %d, want the reloaded 10", v)
	}
}

func TestForPanicsOnInvalidPolicy(t *testing.T) {
	defer func() {
		if err, _ := recover().(error); !errors.Is(err, gashe.ErrInvalidOptions) {
			t.Fatalf("recovered %v, want ErrInvalidOptions", err)
		}
	}()
	gashe.For(newCache(t), gashe.Policy[user]{TTL: "1hr"})
}
