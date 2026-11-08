// Command ratelimit lets a client make five login attempts a minute, then bans it, and limits an
// API key with a sliding window.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:example]
	login := gashe.FixedWindow{
		Calls:  5,
		Per:    time.Minute,
		BanFor: 15 * time.Minute,
	}
	for range 5 {
		c.RateLimit(ctx, "login:1.2.3.4", login) // attempts 1-5: nil, allowed
	}
	err := c.RateLimit(ctx, "login:1.2.3.4", login) // attempt 6: banned for 15m

	var limited *gashe.RateLimitError // errors.Is(err, gashe.ErrRateLimited) holds too
	errors.As(err, &limited)          // limited.RetryAfter is 15m

	for range 3 {
		c.RateLimit(ctx, "api-key:k1", gashe.PerSecond(3)) // calls 1-3: allowed
	}
	c.RateLimit(ctx, "api-key:k1", gashe.PerSecond(3)) // call 4: refused
	// --8<-- [end:example]
}
