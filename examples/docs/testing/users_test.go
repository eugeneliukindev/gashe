//go:build go1.25

package users

import (
	"context"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// newCache gives every test an empty cache of its own.
func newCache(t *testing.T, b gashe.Backend) *gashe.Cache {
	if b == nil {
		b = memory.New(memory.Options{PurgeInterval: -1}) // (1)!
	}
	return gashe.New(b, gashe.Options{
		Hooks: gashe.Hooks{OnError: func(_ context.Context, err error) { t.Log("cache:", err) }},
	})
}

// countingQuery is a fake database that counts its queries.
func countingQuery(n *int) func(context.Context, string) (User, error) {
	return func(_ context.Context, id string) (User, error) {
		*n++
		return User{Name: "user " + id}, nil
	}
}

func TestGetQueriesOnce(t *testing.T) {
	queries := 0
	users := New(newCache(t, nil), countingQuery(&queries))
	for range 3 {
		_, _ = users.Get(t.Context(), "42")
	}
	if queries != 1 {
		t.Fatalf("queries = %d, want 1", queries)
	}
}
