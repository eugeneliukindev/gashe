package gashe_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	cachedUsers := gashe.Policy[User]{TTL: time.Hour}
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

	for range 3 {
		fmt.Println(c.RateLimit(ctx, "user:42", gashe.PerMinute(2)))
	}
	// Output:
	// <nil>
	// <nil>
	// gashe: rate limited
}

func ExampleCache_CircuitBreaker() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	db := gashe.CircuitBreakerOptions{FailurePercent: 50, Window: time.Minute, OpenFor: 30 * time.Second}

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
		OnError: func(ctx context.Context, err error) {
			slog.WarnContext(ctx, "cache degraded", "error", err)
		},
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
		_, err := gashe.GetOrLoad(ctx, c, key, gashe.Policy[Token]{TTL: ttl}, load)
		left, _ := c.TTL(ctx, key)
		fmt.Printf("%s: %v %v\n", key, left.Round(time.Minute), errors.Is(err, gashe.ErrInvalidOptions))
	}
	// Unordered output:
	// duration: 1h0m0s false
	// string: 1h30m0s false
	// func: 4m0s false
	// int: 0s true
}
