// Command ratelimit lets a client make five login attempts a minute, then bans it, and limits an
// API key with a sliding window.
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	login := gashe.FixedWindow{
		Calls:  5,
		Per:    time.Minute,
		BanFor: 15 * time.Minute,
	}
	for attempt := 1; attempt <= 7; attempt++ {
		err := c.RateLimit(ctx, "login:1.2.3.4", login)
		var limited *gashe.RateLimitError // errors.Is(err, gashe.ErrRateLimited) holds too
		if errors.As(err, &limited) {
			// attempts 6-7: banned for 15 minutes
			fmt.Printf("login attempt %d: 429, retry after %v\n", attempt, limited.RetryAfter.Round(time.Second))
			continue
		}
		fmt.Printf("login attempt %d: allowed\n", attempt)
	}

	for call := 1; call <= 4; call++ {
		err := c.RateLimit(ctx, "api-key:k1", gashe.PerSecond(3)) // a sliding window of a second
		var limited *gashe.RateLimitError
		if errors.As(err, &limited) {
			fmt.Printf("api call %d: 429, RetryAfter set for the Retry-After header: %v\n", call, limited.RetryAfter > 0)
			continue
		}
		fmt.Printf("api call %d: allowed\n", call)
	}
}
