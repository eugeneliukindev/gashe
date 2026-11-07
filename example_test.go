package gashe_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct {
	ID   int
	Name string
}

func ExampleGetOrLoad() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	cachedUsers := gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}}
	load := func(context.Context) (User, error) {
		fmt.Println("loading user 42")
		return User{ID: 42, Name: "Ann"}, nil
	}

	for range 2 {
		u, err := gashe.GetOrLoad(ctx, c, "user:42", cachedUsers, load)
		fmt.Println(u.Name, err)
	}
	// Output:
	// loading user 42
	// Ann <nil>
	// Ann <nil>
}

func ExampleCache_Get() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	_ = c.Set(ctx, "key", 90, 2*time.Hour)
	var n int
	err := c.Get(ctx, "key", &n)
	fmt.Println(n, err)

	err = c.Get(ctx, "missing", &n)
	fmt.Println(errors.Is(err, gashe.ErrNotFound))
	// Output:
	// 90 <nil>
	// true
}

func ExampleCache_RateLimit() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	login := gashe.FixedWindow{
		Calls:  2,
		Per:    time.Minute,
		BanFor: 15 * time.Minute,
	}
	for range 3 {
		err := c.RateLimit(ctx, "login:1.2.3.4", login)
		var limited *gashe.RateLimitError
		if errors.As(err, &limited) {
			fmt.Println("429, Retry-After:", limited.RetryAfter) // errors.Is(err, gashe.ErrRateLimited) too
			continue
		}
		fmt.Println(err)
	}
	// Output:
	// <nil>
	// <nil>
	// 429, Retry-After: 15m0s
}

func ExampleCache_CircuitBreaker() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	db := gashe.CircuitBreaker{
		FailurePercent: 50,
		Window:         time.Minute,
		OpenFor:        30 * time.Second,
	}

	for _, callErr := range []error{errors.New("timeout"), nil} {
		done, err := c.CircuitBreaker("db", db)
		if err != nil {
			fmt.Println(err)
			continue
		}
		done(callErr) // report how the real call went
		fmt.Println("called, got", callErr)
	}
	// Output:
	// called, got timeout
	// gashe: circuit open
}

func ExampleOptions_onError() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{
		Hooks: gashe.Hooks{OnError: func(ctx context.Context, err error) {
			slog.WarnContext(ctx, "cache degraded", "error", err)
		}},
	})
	_ = c
	// Output:
}

// TTL takes a time.Duration, a string for time.ParseDuration or a function of the result.
func ExamplePolicy_ttl() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	type Token struct {
		Value     string
		ExpiresIn time.Duration
	}
	load := func(context.Context) (Token, error) { return Token{Value: "t", ExpiresIn: 5 * time.Minute}, nil }

	ttlOf := func(t Token) time.Duration { return t.ExpiresIn - time.Minute }
	for key, ttl := range map[string]any{
		"duration": time.Hour,
		"string":   "1h30m", // such as a value from the configuration
		"func":     ttlOf,   // from the result
		"int":      3600,    // not a time.Duration: an error
	} {
		_, err := gashe.GetOrLoad(ctx, c, key, gashe.Policy[Token]{TTL: gashe.TTL{For: ttl}}, load)
		left, _ := c.TTL(ctx, key)
		fmt.Printf("%s: %v %v\n", key, left.Round(time.Minute), errors.Is(err, gashe.ErrInvalidOptions))
	}
	// Unordered output:
	// duration: 1h0m0s false
	// string: 1h30m0s false
	// func: 4m0s false
	// int: 0s true
}

func ExampleFor() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	users := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "user:"})
	load := func(context.Context) (User, error) {
		fmt.Println("loading user 42")
		return User{ID: 42, Name: "Ann"}, nil
	}

	for range 2 {
		u, err := users.GetOrLoad(ctx, "42", load)
		fmt.Println(u.Name, err)
	}
	// Output:
	// loading user 42
	// Ann <nil>
	// Ann <nil>
}

// Set stores a value right after it was written to the database, so the next read is a hit.
func ExampleCached_Set() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	users := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "user:"})

	_ = users.Set(ctx, "42", User{ID: 42, Name: "Bob"})
	u, err := users.GetOrLoad(ctx, "42", func(context.Context) (User, error) {
		fmt.Println("not called: the key is stored")
		return User{}, nil
	})
	fmt.Println(u.Name, err)
	// Output:
	// Bob <nil>
}

func ExampleCached_Delete() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	users := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "user:"})

	_ = users.Set(ctx, "42", User{ID: 42, Name: "Ann"})
	existed, err := users.Delete(ctx, "42")
	fmt.Println(existed, err)
	existed, err = users.Delete(ctx, "42")
	fmt.Println(existed, err)
	// Output:
	// true <nil>
	// false <nil>
}

func ExampleGetOrLoadMany() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	cachedUsers := gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "user:"}

	_ = gashe.For(c, cachedUsers).Set(ctx, "1", User{ID: 1, Name: "Ann"})
	users, err := gashe.GetOrLoadMany(ctx, c, []string{"1", "2"}, cachedUsers,
		func(_ context.Context, missing []string) (map[string]User, error) {
			fmt.Println("loading", missing) // one query for every missing key
			return map[string]User{"2": {ID: 2, Name: "Bob"}}, nil
		})
	fmt.Println(users["1"].Name, users["2"].Name, err)
	// Output:
	// loading [2]
	// Ann Bob <nil>
}

func ExampleReload() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	cachedUsers := gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}}

	old := func(context.Context) (User, error) { return User{ID: 42, Name: "Ann"}, nil }
	renamed := func(context.Context) (User, error) { return User{ID: 42, Name: "Anna"}, nil }

	u, _ := gashe.GetOrLoad(ctx, c, "user:42", cachedUsers, old)
	fmt.Println(u.Name)
	_, _ = gashe.Reload(ctx, c, "user:42", cachedUsers, renamed) // after the rename in the database
	u, _ = gashe.GetOrLoad(ctx, c, "user:42", cachedUsers, old)
	fmt.Println(u.Name)
	// Output:
	// Ann
	// Anna
}

func ExamplePolicy_staleWhileRevalidate() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	prices := gashe.For(
		c,
		gashe.Policy[int]{TTL: gashe.TTL{For: time.Millisecond}, Stale: gashe.Stale{WhileRevalidate: time.Hour}},
	)

	var version atomic.Int64
	load := func(context.Context) (int, error) { return int(version.Add(1)), nil }

	v, _ := prices.GetOrLoad(ctx, "price", load)
	fmt.Println(v)
	time.Sleep(10 * time.Millisecond) // past TTL: the stored price is stale
	v, _ = prices.GetOrLoad(ctx, "price", load)
	fmt.Println(v, "answered at once, reloading in the background")
	_ = c.Shutdown(ctx) // waits for the background reload
	v, _ = prices.GetOrLoad(ctx, "price", load)
	fmt.Println(v)
	// Output:
	// 1
	// 1 answered at once, reloading in the background
	// 2
}

func ExamplePolicy_staleIfError() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	rates := gashe.For(
		c,
		gashe.Policy[float64]{
			TTL:   gashe.TTL{For: time.Millisecond},
			Stale: gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}},
		},
	)

	v, err := rates.GetOrLoad(ctx, "usd", func(context.Context) (float64, error) { return 1.08, nil })
	fmt.Println(v, err)
	time.Sleep(10 * time.Millisecond) // past TTL: the next call loads
	v, err = rates.GetOrLoad(ctx, "usd", func(context.Context) (float64, error) {
		return 0, errors.New("rates API is down")
	})
	fmt.Println(v, err)
	// Output:
	// 1.08 <nil>
	// 1.08 <nil>
}

func ExamplePolicy_negative() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	errNoUser := errors.New("no such user") // such as sql.ErrNoRows
	users := gashe.For(c, gashe.Policy[User]{
		TTL: gashe.TTL{For: time.Hour},
		Negative: gashe.Negative{
			IsNotFound: func(err error) bool { return errors.Is(err, errNoUser) },
			TTL:        time.Minute,
		},
	})
	load := func(context.Context) (User, error) {
		fmt.Println("querying the database")
		return User{}, errNoUser
	}

	for range 2 {
		_, err := users.GetOrLoad(ctx, "user:404", load)
		fmt.Println(errors.Is(err, gashe.ErrNotFound)) // from the loader, then from the cache
	}
	// Output:
	// querying the database
	// true
	// true
}

func ExamplePolicy_lockOnMiss() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// One process in the cluster builds the report; the others wait up to 30s for its result.
	reports := gashe.For(
		c,
		gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{LockFor: 30 * time.Second}},
	)
	report, err := reports.GetOrLoad(ctx, "report:2026-11", func(context.Context) (string, error) {
		return "monthly report", nil
	})
	fmt.Println(report, err)
	// Output:
	// monthly report <nil>
}

func ExamplePolicy_tags() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	users := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Tags: []string{"users"}})
	load := func(context.Context) (User, error) {
		fmt.Println("loading")
		return User{ID: 42, Name: "Ann"}, nil
	}

	_, _ = users.GetOrLoad(ctx, "user:42", load)
	_, _ = users.GetOrLoad(ctx, "user:42", load) // a hit
	_ = c.InvalidateTags(ctx, "users")           // every result tagged "users" is now a miss
	_, _ = users.GetOrLoad(ctx, "user:42", load)
	// Output:
	// loading
	// loading
}

func ExamplePolicy_maxConcurrentLoads() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	reports := gashe.For(c, gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}, Stampede: gashe.Stampede{MaxLoads: 1}})

	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = reports.GetOrLoad(ctx, "a", func(context.Context) (string, error) {
			close(started)
			<-release // a slow query holds the only slot
			return "a", nil
		})
	}()
	<-started

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_, err := reports.GetOrLoad(waitCtx, "b", func(context.Context) (string, error) { return "b", nil })
	fmt.Println(errors.Is(err, gashe.ErrOverloaded), err)
	close(release)
	<-done
	// Output:
	// true gashe: overloaded: context deadline exceeded
}

// Fallback keeps results in a memory backend of its own; while the backend of the cache fails,
// calls are answered from it instead of loading every time.
func ExamplePolicy_fallback() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	spare := memory.New(memory.Options{})
	defer spare.Close()
	c := gashe.New(mem, gashe.Options{})

	users := gashe.For(c, gashe.Policy[User]{
		TTL:      gashe.TTL{For: time.Hour},
		Fallback: gashe.Fallback{Backend: spare, MaxTTL: time.Minute},
	})
	u, err := users.GetOrLoad(ctx, "user:42", func(context.Context) (User, error) {
		return User{ID: 42, Name: "Ann"}, nil
	})
	fmt.Println(u.Name, err)
	// Output:
	// Ann <nil>
}

func ExampleCache_WithLock() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// One process at a time runs the job; the lock is extended while it runs.
	err := c.WithLock(ctx, "job:invoices", time.Minute, func(context.Context) error {
		fmt.Println("sending invoices")
		return nil
	})
	fmt.Println(err)
	// Output:
	// sending invoices
	// <nil>
}

func ExampleCache_TryLock() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	lease, ok, err := c.TryLock(ctx, "job:invoices", time.Minute)
	fmt.Println(ok, err)
	_, ok, err = c.TryLock(ctx, "job:invoices", time.Minute) // held: skip the job
	fmt.Println(ok, err)
	_ = lease.Unlock(ctx)
	// Output:
	// true <nil>
	// false <nil>
}

func ExampleGuard() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	payments := gashe.CircuitBreaker{FailurePercent: 50, Window: time.Minute, OpenFor: 30 * time.Second}

	for range 2 {
		id, err := gashe.Guard(c, "payments", payments, func() (string, error) {
			return "", errors.New("timeout")
		})
		fmt.Printf("%q %v\n", id, err)
	}
	// Output:
	// "" timeout
	// "" gashe: circuit open
}

func ExampleOptions_prefix() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{Prefix: "orders:"}) // several services share one Redis

	_ = c.Set(ctx, "user:42", "Ann", time.Hour)
	stored, _ := mem.Exists(ctx, "orders:user:42")
	fmt.Println(stored)
	for key, err := range c.Keys(ctx, "user:*") {
		fmt.Println(key, err)
	}
	// Output:
	// true
	// user:42 <nil>
}

func ExampleCache_IncrBy() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	for range 2 {
		n, err := c.IncrBy(ctx, "views:home", 1, 24*time.Hour) // the TTL applies on creation only
		fmt.Println(n, err)
	}
	var n int64
	_ = c.Get(ctx, "views:home", &n)
	fmt.Println(n)
	// Output:
	// 1 <nil>
	// 2 <nil>
	// 2
}

func ExampleCache_Keys() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	_ = c.SetMany(ctx, map[string]any{"user:1": "Ann", "user:2": "Bob", "order:1": 100}, time.Hour)
	for key, err := range c.Keys(ctx, "user:*") {
		fmt.Println(key, err)
	}
	// Unordered output:
	// user:1 <nil>
	// user:2 <nil>
}
