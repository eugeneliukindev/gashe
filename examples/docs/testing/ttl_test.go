package users

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestGetQueriesAgainAfterTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) { // (1)!
		queries := 0
		users := New(newCache(t, nil), countingQuery(&queries))
		_, _ = users.Get(t.Context(), "42")
		time.Sleep(time.Hour + time.Second)
		_, _ = users.Get(t.Context(), "42")
		if queries != 2 {
			t.Fatalf("queries = %d, want 2", queries)
		}
	})
}
