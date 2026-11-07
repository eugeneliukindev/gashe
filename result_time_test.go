//go:build go1.25

package gashe_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func TestOutcomeTimesTheLoader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		mem := memory.New(memory.Options{PurgeInterval: -1})
		t.Cleanup(func() { _ = mem.Close() })
		var got []gashe.Outcome
		c := gashe.New(
			mem,
			gashe.Options{
				Hooks: gashe.Hooks{OnResult: func(_ context.Context, o gashe.Outcome) { got = append(got, o) }},
			},
		)
		slow := func(context.Context) (int, error) { time.Sleep(time.Second); return 1, nil }
		o := gashe.Policy[int]{TTL: gashe.TTL{For: time.Hour}, Prefix: "n:"}

		_, _ = gashe.GetOrLoad(ctx, c, "1", o, slow)
		_, _ = gashe.GetOrLoad(ctx, c, "1", o, slow)
		_, _ = gashe.GetOrLoadMany(ctx, c, []string{"1", "2"}, o,
			func(context.Context, []string) (map[string]int, error) { time.Sleep(2 * time.Second); return nil, nil })

		want := []gashe.Outcome{
			{Key: "n:1", Result: gashe.ResultMiss, Load: time.Second},
			{Key: "n:1", Result: gashe.ResultHit},
			{Key: "n:1", Result: gashe.ResultHit},
			{Key: "n:2", Result: gashe.ResultMiss, Load: 2 * time.Second},
		}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("outcome %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})
}
