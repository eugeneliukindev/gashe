package gashe_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/memory"
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

	users := gashe.CacheOptions[User]{TTL: time.Hour}
	load := func(context.Context) (User, error) {
		fmt.Println("loading user 42")
		return User{ID: 42, Name: "Ann"}, nil
	}

	for range 2 {
		u, err := gashe.GetOrLoad(ctx, c, "user:42", users, load)
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
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	db := gashe.BreakerOptions{FailurePercent: 50, Window: time.Minute, OpenFor: 30 * time.Second}

	for _, callErr := range []error{errors.New("timeout"), nil} {
		done, err := c.CircuitBreaker(ctx, "db", db)
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
