//go:build go1.25

package gashe_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestFallbackMaxTTLOnlyShortens(t *testing.T) {
	for _, tc := range []struct {
		name string
		ttl  time.Duration
		kept time.Duration // how long the fallback answers
	}{
		{"a long TTL is cut to MaxTTL", time.Hour, 30 * time.Second},
		{"a short TTL stays", 10 * time.Second, 10 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				c, fb, _ := withFallback(t, 30*time.Second)
				calls, load := counter(user{Name: "Ann"})
				o := gashe.Policy[user]{TTL: gashe.TTL{For: tc.ttl}, Fallback: fb}

				_, _ = gashe.GetOrLoad(ctx, c, "42", o, load)
				time.Sleep(tc.kept - time.Second)
				_, _ = gashe.GetOrLoad(ctx, c, "42", o, load)
				if *calls != 1 {
					t.Fatalf("loaded %d times within %v, want once", *calls, tc.kept)
				}
				time.Sleep(2 * time.Second)
				_, _ = gashe.GetOrLoad(ctx, c, "42", o, load)
				if *calls != 2 {
					t.Fatalf("loaded %d times after %v, want the entry gone and a second load", *calls, tc.kept)
				}
			})
		})
	}
}
