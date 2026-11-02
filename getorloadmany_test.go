package gashe_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// manyDown fails every GetMany and SetMany, like an unreachable Redis.
type manyDown struct {
	gashe.Backend

	setManys int
}

func (*manyDown) GetMany(context.Context, ...string) ([][]byte, error) { return nil, errBackendDown }

func (d *manyDown) SetMany(context.Context, map[string][]byte, time.Duration) error {
	d.setManys++
	return errBackendDown
}

// byName loads users named after their keys, records what it was asked for, and leaves out "ghost".
func byName(asked *[][]string) func(context.Context, []string) (map[string]user, error) {
	return func(_ context.Context, missing []string) (map[string]user, error) {
		*asked = append(*asked, missing)
		out := make(map[string]user, len(missing))
		for _, k := range missing {
			if k != "ghost" {
				out[k] = user{Name: k}
			}
		}
		return out, nil
	}
}

func TestGetOrLoadManyLoadsOnlyMissing(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.Policy[user]{TTL: time.Minute, Prefix: "user:"}
	var asked [][]string
	load := byName(&asked)

	got, err := gashe.GetOrLoadMany(ctx, c, []string{"b", "a", "ghost", "a"}, o, load)
	if err != nil || len(got) != 2 || got["a"].Name != "a" || got["b"].Name != "b" {
		t.Fatalf("first call: %v, %v", got, err)
	}
	got, err = gashe.GetOrLoadMany(ctx, c, []string{"a", "b", "c", "ghost"}, o, load)
	if err != nil || len(got) != 3 || got["c"].Name != "c" {
		t.Fatalf("second call: %v, %v", got, err)
	}
	want := [][]string{{"a", "b", "ghost"}, {"c", "ghost"}}
	if !slices.EqualFunc(asked, want, slices.Equal[[]string]) {
		t.Fatalf("loader asked for %v, want %v", asked, want)
	}
	// GetOrLoad reads what GetOrLoadMany stored under the same policy.
	u, err := gashe.GetOrLoad(ctx, c, "a", o, func(context.Context) (user, error) {
		t.Fatal("GetOrLoad reloaded a stored key")
		return user{}, nil
	})
	if err != nil || u.Name != "a" {
		t.Fatalf("GetOrLoad: %v, %v", u, err)
	}
}

func TestGetOrLoadManyTags(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.Policy[user]{TTL: time.Minute, Tags: []string{"users"}}
	var asked [][]string
	load := byName(&asked)

	_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a"}, o, load)
	_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a"}, o, load)
	if err := c.InvalidateTags(ctx, "users"); err != nil {
		t.Fatal(err)
	}
	_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a"}, o, load)
	if len(asked) != 2 {
		t.Fatalf("loads = %v, want one before and one after the invalidation", asked)
	}
}

func TestGetOrLoadManyCacheIf(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.Policy[user]{TTL: time.Minute, CacheIf: func(u user) bool { return u.Name != "a" }}
	var asked [][]string
	load := byName(&asked)

	_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a", "b"}, o, load)
	_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a", "b"}, o, load)
	if len(asked) != 2 || !slices.Equal(asked[1], []string{"a"}) {
		t.Fatalf("loader asked for %v; only a must be loaded again", asked)
	}
}

func TestGetOrLoadManyBackendDown(t *testing.T) {
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	down := &manyDown{Backend: mem}
	var reported []error
	c := gashe.New(
		down,
		gashe.Options{OnError: func(_ context.Context, err error) { reported = append(reported, err) }},
	)
	var asked [][]string

	got, err := gashe.GetOrLoadMany(context.Background(), c, []string{"a", "b"}, gashe.Policy[user]{TTL: time.Minute},
		byName(&asked))
	if err != nil || len(got) != 2 || down.setManys != 0 {
		t.Fatalf("got %v, %v, %d writes; want the loader result and no write", got, err, down.setManys)
	}
	if len(reported) != 1 || !errors.Is(reported[0], errBackendDown) {
		t.Fatalf("reported %v", reported)
	}
}

func TestGetOrLoadManyErrors(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	var asked [][]string
	if _, err := gashe.GetOrLoadMany(ctx, c, []string{"a"},
		gashe.Policy[user]{TTL: time.Minute, StaleWhileRevalidate: time.Minute}, byName(&asked)); !errors.Is(
		err, gashe.ErrInvalidOptions) {
		t.Fatalf("StaleWhileRevalidate: %v, want ErrInvalidOptions", err)
	}
	failing := func(context.Context, []string) (map[string]user, error) { return nil, errAPI }
	if _, err := gashe.GetOrLoadMany(ctx, c, []string{"a"}, gashe.Policy[user]{TTL: time.Minute}, failing); !errors.Is(
		err, errAPI) {
		t.Fatalf("loader error: %v", err)
	}
	if got, err := gashe.GetOrLoadMany(ctx, c, nil, gashe.Policy[user]{TTL: time.Minute}, failing); err != nil ||
		len(got) != 0 {
		t.Fatalf("no keys: %v, %v", got, err)
	}
}
