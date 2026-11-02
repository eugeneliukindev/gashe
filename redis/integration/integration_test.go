package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backendtest"
	gasheredis "github.com/eugeneliukindev/gashe/redis"
)

// redisImage is the oldest Redis the backend is tested against.
const redisImage = "redis:7.0.0"

// addr is the address of the Redis container that TestMain starts for the whole package.
var addr string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        redisImage,
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration tests need Docker:", err)
		return 1
	}
	defer func() { _ = container.Terminate(ctx) }()
	if addr, err = container.Endpoint(ctx, ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func newClient(t *testing.T) *goredis.Client {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: addr})
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

func TestGetOrLoadOverRedis(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.CacheOptions[string]{
		TTL:            time.Hour,
		RefreshWindow:  time.Hour,
		FallbackWindow: time.Hour,
		LockOnMiss:     time.Second,
		MaxReads:       2,
	}
	calls := 0
	load := func(context.Context) (string, error) {
		calls++
		return "Ann", nil
	}
	for i, wantCalls := range []int{1, 1, 1, 2} { // a load, two reads, then MaxReads reloads
		if got, err := gashe.GetOrLoad(ctx, c, "user:1", users, load); err != nil || got != "Ann" {
			t.Fatalf("call %d: %q, %v", i+1, got, err)
		}
		if calls != wantCalls {
			t.Fatalf("call %d: loader ran %d times, want %d", i+1, calls, wantCalls)
		}
	}
}

func TestLocksAndLimitsOverRedis(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)

	unlock, ok, lockErr := c.TryLock(ctx, "job", time.Minute)
	if lockErr != nil || !ok {
		t.Fatalf("TryLock = %v, %v", ok, lockErr)
	}
	if _, ok, _ := c.TryLock(ctx, "job", time.Minute); ok {
		t.Fatal("a held lock was taken twice")
	}
	if err := unlock(ctx); err != nil {
		t.Fatal(err)
	}

	for _, limit := range []gashe.Limit{gashe.Fixed{Calls: 2, Per: time.Minute}, gashe.PerMinute(2)} {
		for i := range 2 {
			if err := c.RateLimit(ctx, "k", limit); err != nil {
				t.Fatalf("%T call %d: %v", limit, i+1, err)
			}
		}
		if err := c.RateLimit(ctx, "k", limit); !errors.Is(err, gashe.ErrRateLimited) {
			t.Fatalf("%T over the limit: %v", limit, err)
		}
	}

	db := gashe.BreakerOptions{FailurePercent: 50, Window: time.Minute, OpenFor: time.Minute, TrialCalls: 1}
	done, err := c.CircuitBreaker(ctx, "db", db)
	if err != nil {
		t.Fatal(err)
	}
	done(errors.New("boom"))
	if _, err := c.CircuitBreaker(ctx, "db", db); !errors.Is(err, gashe.ErrCircuitOpen) {
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
