package gashe

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	// slidingWindows is how many fixed windows a sliding limit reads: the current and the last.
	slidingWindows = 2
)

// Limit is a rate limit for Cache.RateLimit: SlidingWindow or FixedWindow.
type Limit interface {
	allow(ctx context.Context, c *Cache, key string) error
}

// SlidingWindow allows about Calls calls in any span of Per. It keeps two counters per key, for the
// current and the last fixed window of Per, and weighs the last one by how much of it still
// falls inside the sliding window, as if its calls were spread evenly. Refused calls are not
// counted.
type SlidingWindow struct {
	// Calls is how many calls the window allows; it must be positive.
	Calls int
	// Per is the span of the window; it must be positive.
	Per time.Duration
}

// PerSecond allows n calls in any second.
func PerSecond(n int) SlidingWindow { return SlidingWindow{Calls: n, Per: time.Second} }

// PerMinute allows n calls in any minute.
func PerMinute(n int) SlidingWindow { return SlidingWindow{Calls: n, Per: time.Minute} }

// PerHour allows n calls in any hour.
func PerHour(n int) SlidingWindow { return SlidingWindow{Calls: n, Per: time.Hour} }

// FixedWindow allows Calls calls per window of Per that starts with the first call. The first call
// over the limit blocks the key for BanFor, counted from that call; 0 means Per.
type FixedWindow struct {
	// Calls is how many calls a window allows; it must be positive.
	Calls int
	// Per is the span of a window, from its first call; it must be positive.
	Per time.Duration
	// BanFor is how long the first call over the limit blocks the key, counted from that call. 0 means
	// Per.
	BanFor time.Duration
}

// RateLimitError is what RateLimit returns over the limit; errors.Is(err, ErrRateLimited) holds.
type RateLimitError struct {
	// RetryAfter is how long until a call would pass again if no other calls come, for a
	// Retry-After header.
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return "gashe: rate limited, retry after " + e.RetryAfter.String()
}

// Is reports whether target is ErrRateLimited.
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// RateLimit counts a call for key and returns a *RateLimitError, which is ErrRateLimited, once the
// key is over limit, across every process that uses the same backend. Call it before the work it guards. If the backend
// fails, it reports the error to Options.OnError and lets the call through.
func (c *Cache) RateLimit(ctx context.Context, key string, limit Limit) error {
	return limit.allow(ctx, c, key)
}

func (l SlidingWindow) allow(ctx context.Context, c *Cache, key string) error {
	if l.Calls <= 0 || l.Per <= 0 {
		return fmt.Errorf("%w: SlidingWindow needs positive Calls and Per", ErrInvalidOptions)
	}
	key = ownSliding + key
	now := time.Now()
	window := now.UnixNano() / int64(l.Per)
	current := key + ":" + strconv.FormatInt(window, 10)
	n, err := c.backend.IncrBy(ctx, current, 1, slidingWindows*l.Per)
	if err != nil {
		c.onError(ctx, fmt.Errorf("gashe: rate limit %q: %w", key, err))
		return nil
	}
	last, err := readCount(ctx, c, key+":"+strconv.FormatInt(window-1, 10))
	if err != nil {
		c.onError(ctx, fmt.Errorf("gashe: rate limit %q: %w", key, err))
	}
	overlap := 1 - float64(now.UnixNano()%int64(l.Per))/float64(l.Per)
	if float64(last)*overlap+float64(n) <= float64(l.Calls) {
		return nil
	}
	if _, err := c.backend.IncrBy(ctx, current, -1, 0); err != nil {
		c.onError(ctx, fmt.Errorf("gashe: rate limit %q: %w", key, err))
	}
	return &RateLimitError{RetryAfter: l.retryAfter(last, n-1, now)}
}

// retryAfter returns how long until a call passes, given the calls of the last and the current
// window and no calls to come: until the last window weighs little enough, after the current one
// ends if it is full by itself.
func (l SlidingWindow) retryAfter(last, current int64, now time.Time) time.Duration {
	per := float64(l.Per)
	elapsed := float64(now.UnixNano() % int64(l.Per))
	room := float64(l.Calls) - 1 // what the weighted calls may sum to before this one
	if float64(current) > room {
		// The current window becomes the last one; wait until it weighs room at most.
		return time.Duration(per - elapsed + max(0, per*(1-room/float64(current))))
	}
	// share is how much of the last window may still overlap.
	share := (room - float64(current)) / float64(last)
	return time.Duration(max(0, per*(1-share)-elapsed))
}

func (l FixedWindow) allow(ctx context.Context, c *Cache, key string) error {
	if l.Calls <= 0 || l.Per <= 0 || l.BanFor < 0 {
		return fmt.Errorf("%w: FixedWindow needs positive Calls and Per and a non-negative BanFor", ErrInvalidOptions)
	}
	key = ownFixed + key
	n, err := c.backend.IncrBy(ctx, key, 1, l.Per)
	if err != nil {
		c.onError(ctx, fmt.Errorf("gashe: rate limit %q: %w", key, err))
		return nil
	}
	if n <= int64(l.Calls) {
		return nil
	}
	ban := cmp.Or(l.BanFor, l.Per)
	if n == int64(l.Calls)+1 {
		if _, err := c.backend.Expire(ctx, key, ban); err != nil {
			c.onError(ctx, fmt.Errorf("gashe: rate limit ban %q: %w", key, err))
		}
		return &RateLimitError{RetryAfter: ban}
	}
	return &RateLimitError{RetryAfter: banLeft(ctx, c, key, ban)}
}

// banLeft returns what is left of the ban on key, or all of ban if that cannot be read.
func banLeft(ctx context.Context, c *Cache, key string, ban time.Duration) time.Duration {
	left, err := c.backend.TTL(ctx, key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		c.onError(ctx, fmt.Errorf("gashe: rate limit ban %q: %w", key, err))
	}
	if err != nil || left < 0 {
		return ban // the counter is gone or has no TTL: the longest it can be
	}
	return left
}

// readCount reads a counter written by IncrBy; a missing counter is 0.
func readCount(ctx context.Context, c *Cache, key string) (int64, error) {
	raw, err := c.backend.Get(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrWrongType, err)
	}
	return n, nil
}
