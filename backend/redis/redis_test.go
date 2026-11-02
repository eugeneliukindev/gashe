package redis_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

// Tests against a real Redis live in the integration module next to this one.

func TestUnreachableServer(t *testing.T) {
	ctx := context.Background()
	// Port 1 refuses connections at once; no retries keeps the test free of backoff sleeps.
	client := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	b := gasheredis.New(client)

	_, err := b.Get(ctx, "k")
	if err == nil || errors.Is(err, gashe.ErrNotFound) || !strings.HasPrefix(err.Error(), "gashe/redis: get: ") {
		t.Fatalf("Get = %v, want a wrapped connection error", err)
	}

	var reported []error
	c := gashe.New(b, gashe.Options{OnError: func(_ context.Context, err error) { reported = append(reported, err) }})
	got, err := gashe.GetOrLoad(ctx, c, "k", gashe.Policy[string]{TTL: time.Hour},
		func(context.Context) (string, error) { return "fresh", nil })
	if err != nil || got != "fresh" {
		t.Fatalf("Do = %q, %v; want the loader result", got, err)
	}
	if len(reported) != 1 {
		t.Fatalf("OnError got %v, want one error", reported)
	}
}
