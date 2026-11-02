//go:build go1.25

package gashe_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestGetOrLoadRefreshAfterReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newCached(newCache(t), gashe.CacheOptions[int]{TTL: time.Hour, MaxReads: 10, RefreshAfterReads: 2})
		_, load := versions()

		// load, read 1, read 2 refreshes in the background, then the new value
		for i, want := range []int{1, 1, 1, 2} {
			if got, _ := s.Do(ctx, "k", load); got != want {
				t.Fatalf("call %d: v%d, want v%d", i+1, got, want)
			}
			synctest.Wait()
		}
	})
}
