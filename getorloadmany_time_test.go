//go:build go1.25

package gashe_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestGetOrLoadManyExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := newCache(t)
		o := gashe.Policy[user]{TTL: gashe.TTL{For: time.Minute, Jitter: 0.1}}
		var asked [][]string
		load := byName(&asked)

		_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a", "b"}, o, load)
		time.Sleep(50 * time.Second)
		_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a", "b"}, o, load)
		time.Sleep(20 * time.Second) // past TTL plus Jitter
		_, _ = gashe.GetOrLoadMany(ctx, c, []string{"a", "b"}, o, load)
		if len(asked) != 2 || len(asked[1]) != 2 {
			t.Fatalf("loader asked for %v; want both keys again after TTL", asked)
		}
	})
}
