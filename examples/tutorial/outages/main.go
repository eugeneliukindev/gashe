// Command outages is step 6 of the tutorial: Redis goes down and the program keeps answering.
// Options.CircuitBreaker stops calling Redis, and Policy.Fallback answers from memory, so the
// database gets one query. A memory backend that can fail stands in for Redis.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct {
	ID   string
	Name string
}

// flaky stands for Redis: while down, every call fails, as with a lost connection.
type flaky struct {
	gashe.Backend

	down  bool
	calls int // calls that reached Redis while it was down
}

var errDown = errors.New("redis: connection refused")

func (f *flaky) Get(ctx context.Context, key string) ([]byte, error) {
	if f.down {
		f.calls++
		return nil, errDown
	}
	return f.Backend.Get(ctx, key)
}

func (f *flaky) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if f.down {
		f.calls++
		return errDown
	}
	return f.Backend.Set(ctx, key, value, ttl)
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	redis := &flaky{Backend: mem}
	results := map[gashe.Result]int{} // how the calls were answered
	c := gashe.New(redis, gashe.Options{
		CircuitBreaker: &gashe.CircuitBreakerOptions{
			FailurePercent: 50,          // open once half of the calls fail
			MinCalls:       4,           // ... out of at least 4
			Window:         time.Minute, // counted over the last minute
			OpenFor:        time.Minute, // then do not call Redis for a minute
		},
		OnResult: func(_ context.Context, o gashe.Outcome) { results[o.Result]++ },
	})

	spare := memory.New(memory.Options{MaxBytes: 32 << 20}) // memory of this pod, for while Redis is down
	defer spare.Close()
	users := gashe.For(c, gashe.Policy[User]{
		TTL:      time.Hour,
		Fallback: &gashe.FallbackOptions{Backend: spare, MaxTTL: 30 * time.Second},
		Prefix:   "user:",
	})

	queries := 0
	loadUser := func(context.Context) (User, error) {
		queries++
		return User{ID: "42", Name: "Ann"}, nil
	}

	redis.down = true
	for range 10 {
		_, err := users.GetOrLoad(ctx, "42", loadUser)
		check(err)
	}
	fmt.Println("Redis is down: 10 requests")
	fmt.Println("  miss:", results[gashe.ResultMiss])
	fmt.Println("  fallback:", results[gashe.ResultFallback])
	fmt.Println("database queries:", queries)
	fmt.Println("calls to Redis:", redis.calls) // 10 without the breaker
}

// check stops the program on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
