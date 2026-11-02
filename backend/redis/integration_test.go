package redis_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
	"github.com/eugeneliukindev/gashe/backendtest"
)

// redisAddr returns the address of the Redis these tests run against, from GASHE_REDIS_ADDR.
// `just integration` starts one in Docker and sets it; without it the tests are skipped, so
// go test needs no Docker.
func redisAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("GASHE_REDIS_ADDR")
	if addr == "" {
		t.Skip("no Redis: GASHE_REDIS_ADDR is not set; run `just integration`")
	}
	return addr
}

func newClient(t *testing.T) *goredis.Client {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: redisAddr(t)})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestContract(t *testing.T) {
	client := newClient(t)
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		t.Helper()
		if err := client.FlushDB(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		return gasheredis.New(client)
	})
}

func TestWrappersOverRedis(t *testing.T) {
	client := newClient(t)
	fresh := func(t *testing.T) gashe.Backend {
		t.Helper()
		if err := client.FlushDB(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		return gasheredis.New(client)
	}
	t.Run("Prefix", func(t *testing.T) {
		backendtest.Run(t, func(t *testing.T) gashe.Backend {
			t.Helper()
			return gashe.New(fresh(t), gashe.Options{Prefix: "svc:"}).Backend()
		})
	})
	t.Run("NearCache", func(t *testing.T) {
		backendtest.Run(t, func(t *testing.T) gashe.Backend {
			t.Helper()
			local := memory.New(memory.Options{PurgeInterval: -1})
			t.Cleanup(func() { _ = local.Close() })
			return nearcache.New(local, fresh(t), nearcache.Options{})
		})
	})
}

func TestGetOrLoadOverRedis(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.Policy[string]{
		TTL:                  time.Hour,
		StaleWhileRevalidate: time.Hour,
		StaleIfError:         time.Hour,
		LockOnMiss:           time.Second,
		Tags:                 []string{"users"},
	}
	calls := 0
	load := func(context.Context) (string, error) {
		calls++
		return "Ann", nil
	}
	for i, wantCalls := range []int{1, 1, 2} { // a load, a hit, then a reload after InvalidateTags
		if i == 2 {
			if err := c.InvalidateTags(ctx, "users"); err != nil {
				t.Fatal(err)
			}
		}
		if got, err := gashe.GetOrLoad(ctx, c, "user:1", users, load); err != nil || got != "Ann" {
			t.Fatalf("call %d: %q, %v", i+1, got, err)
		}
		if calls != wantCalls {
			t.Fatalf("call %d: loader ran %d times, want %d", i+1, calls, wantCalls)
		}
	}
}

func TestGetOrLoadManyOverRedis(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.Policy[string]{TTL: time.Hour, Prefix: "user:", Tags: []string{"users"}}
	var asked [][]string
	load := func(_ context.Context, missing []string) (map[string]string, error) {
		asked = append(asked, missing)
		out := make(map[string]string, len(missing))
		for _, k := range missing {
			out[k] = "name " + k
		}
		return out, nil
	}
	for _, keys := range [][]string{{"1", "2"}, {"1", "2", "3"}} {
		got, err := gashe.GetOrLoadMany(ctx, c, keys, users, load)
		if err != nil || len(got) != len(keys) || got["1"] != "name 1" {
			t.Fatalf("%v: %v, %v", keys, got, err)
		}
	}
	if len(asked) != 2 || len(asked[1]) != 1 || asked[1][0] != "3" {
		t.Fatalf("loader asked for %v; the second call must load only 3", asked)
	}
	ttl, err := c.TTL(ctx, "user:1")
	if err != nil || ttl <= 0 || ttl > time.Hour {
		t.Fatalf("TTL of a stored entry = %v, %v", ttl, err)
	}
}

func TestLocksAndLimitsOverRedis(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)

	lease, ok, lockErr := c.TryLock(ctx, "job", time.Minute)
	if lockErr != nil || !ok {
		t.Fatalf("TryLock = %v, %v", ok, lockErr)
	}
	if _, ok, _ := c.TryLock(ctx, "job", time.Minute); ok {
		t.Fatal("a held lock was taken twice")
	}
	if err := lease.Extend(ctx, time.Hour); err != nil {
		t.Fatalf("Extend: %v", err)
	}
	if err := lease.Unlock(ctx); err != nil {
		t.Fatal(err)
	}

	for _, limit := range []gashe.Limit{gashe.FixedWindow{Calls: 2, Per: time.Minute}, gashe.PerMinute(2)} {
		for i := range 2 {
			if err := c.RateLimit(ctx, "k", limit); err != nil {
				t.Fatalf("%T call %d: %v", limit, i+1, err)
			}
		}
		if err := c.RateLimit(ctx, "k", limit); !errors.Is(err, gashe.ErrRateLimited) {
			t.Fatalf("%T over the limit: %v", limit, err)
		}
	}

	db := gashe.CircuitBreakerOptions{FailurePercent: 50, Window: time.Minute, OpenFor: time.Minute, HalfOpenCalls: 1}
	done, err := c.CircuitBreaker("db", db)
	if err != nil {
		t.Fatal(err)
	}
	done(errors.New("boom"))
	if _, err := c.CircuitBreaker("db", db); !errors.Is(err, gashe.ErrCircuitOpen) {
		t.Fatalf("after a failure: %v, want ErrCircuitOpen", err)
	}
}

func newCache(t *testing.T) *gashe.Cache {
	t.Helper()
	client := newClient(t)
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	return gashe.New(gasheredis.New(client), gashe.Options{})
}
