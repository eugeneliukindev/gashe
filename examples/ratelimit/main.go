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
	// attempts 1-5: allowed; 6-7: 429, retry after 15m0s
	for attempt := 1; attempt <= 7; attempt++ {
		fmt.Printf("login attempt %d: %s\n", attempt, verdict(c.RateLimit(ctx, "login:1.2.3.4", login)))
	}

	// calls 1-3: allowed; 4: 429, retry after a second or less
	for call := 1; call <= 4; call++ {
		fmt.Printf("api call %d: %s\n", call, verdict(c.RateLimit(ctx, "api-key:k1", gashe.PerSecond(3))))
	}
}

func verdict(err error) string {
	var limited *gashe.RateLimitError // errors.Is(err, gashe.ErrRateLimited) holds too
	switch {
	case errors.As(err, &limited):
		return "429, retry after " + limited.RetryAfter.Round(time.Second).String()
	case err != nil:
		return err.Error()
	}
	return "allowed"
}
