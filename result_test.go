package gashe_test

import (
	"context"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestGetOrLoadOnResult(t *testing.T) {
	ctx := context.Background()
	var got []gashe.Result
	c := gashe.New(newCache(t).Backend(), gashe.Options{
		OnResult: func(_ context.Context, _ string, r gashe.Result) { got = append(got, r) },
	})
	o := gashe.Policy[int]{TTL: time.Hour, StaleIfError: time.Hour}
	_, load := versions()

	_, _ = gashe.GetOrLoad(ctx, c, "k", o, load) // miss
	_, _ = gashe.GetOrLoad(ctx, c, "k", o, load) // hit
	if len(got) != 2 || got[0] != gashe.ResultMiss || got[1] != gashe.ResultHit {
		t.Fatalf("OnResult got %v, want [miss hit]", got)
	}
	if gashe.ResultStaleIfError.String() != "stale-if-error" {
		t.Fatalf("String = %q", gashe.ResultStaleIfError.String())
	}
}
