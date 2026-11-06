# Test code that uses gashe { #test-code-that-uses-gashe }

Test your own code that caches: an empty cache per test, TTLs in fake time, a backend that is down,
and your own backend.

## The code { #the-code }

The code under test, `users.go`, takes the cache and the query, so a test passes both:

```go
package users

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
)

type User struct{ Name string }

// Users is your code: it takes the cache and the query, so a test passes both.
type Users struct {
	cache gashe.Cached[User]
	query func(ctx context.Context, id string) (User, error)
}

func New(c *gashe.Cache, query func(ctx context.Context, id string) (User, error)) *Users {
	return &Users{cache: gashe.For(c, gashe.Policy[User]{TTL: time.Hour, Prefix: "user:"}), query: query}
}

func (u *Users) Get(ctx context.Context, id string) (User, error) {
	return u.cache.GetOrLoad(ctx, id, func(ctx context.Context) (User, error) { return u.query(ctx, id) })
}
```

The tests, `users_test.go` in the same package: they use `User` and `New` from `users.go`.
`testing/synctest` needs Go 1.25 or newer:

```go
package users

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backendtest"
)

// newCache gives every test an empty cache of its own.
func newCache(t *testing.T, b gashe.Backend) *gashe.Cache {
	t.Helper()
	if b == nil {
		b = memory.New(memory.Options{PurgeInterval: -1}) // no purge goroutine: nothing to close
	}
	return gashe.New(b, gashe.Options{
		OnError: func(_ context.Context, err error) { t.Log("cache:", err) },
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
		if _, err := users.Get(t.Context(), "42"); err != nil {
			t.Fatal(err)
		}
	}
	if queries != 1 {
		t.Fatalf("queries = %d, want 1", queries)
	}
}

func TestGetQueriesAgainAfterTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) { // fake time: the hour passes at once
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

var errDown = errors.New("redis: connection refused")

// downBackend is a backend that is down: embed a real one, override what must fail.
type downBackend struct{ gashe.Backend }

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
func (downBackend) GetMany(context.Context, ...string) ([][]byte, error) {
	return nil, errDown
}
func (downBackend) Set(context.Context, string, []byte, time.Duration) error { return errDown }

func TestGetWorksWhileTheCacheIsDown(t *testing.T) {
	queries := 0
	users := New(newCache(t, downBackend{memory.New(memory.Options{PurgeInterval: -1})}), countingQuery(&queries))
	u, err := users.Get(t.Context(), "42")
	if err != nil || u.Name != "user 42" {
		t.Fatalf("Get = %v, %v; want the user from the database", u, err)
	}
}

// loggingBackend is your own backend; here it wraps memory to count reads.
type loggingBackend struct {
	gashe.Backend
	reads atomic.Int64 // a backend must be safe for concurrent use
}

func (b *loggingBackend) Get(ctx context.Context, key string) ([]byte, error) {
	b.reads.Add(1)
	return b.Backend.Get(ctx, key)
}

func TestLoggingBackendKeepsTheContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return &loggingBackend{Backend: memory.New(memory.Options{PurgeInterval: -1})} // empty for every subtest
	})
}
```

## How it works { #how-it-works }

| Test | How |
|---|---|
| one query for many calls | `backend/memory` with `PurgeInterval: -1`: an empty cache per test, no goroutine to stop |
| expiry after `TTL` | `synctest.Test` runs in fake time: `time.Sleep(time.Hour + time.Second)` returns at once, and the memory backend sees the TTL pass |
| the cache backend is down | `downBackend`, the failing backend of [testing your code](../concepts/backends.md#testing-your-code): embed a real backend and override what must fail, here `Get`, `GetMany` and `Set`, so no read or write reaches it; `GetOrLoad` still answers from the loader |
| your own backend | `backendtest.Run` checks the contract the bundled backends pass; give it an empty backend per subtest; [your own backend](../concepts/backends.md#your-own-backend) |
| what the cache worked around | `OnError: t.Log` shows it in the output of a failed test |

## Variations { #variations }

| If you need | Do |
|---|---|
| to check what a `StaleWhileRevalidate` reload stored | call `synctest.Wait()` inside the bubble first: it waits for the background reload |
| to check exact TTLs | leave `Jitter` out of the policy |
| only reads to fail | override `Get` and `GetMany` alone; writes then reach the real backend |
