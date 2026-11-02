package gashe

import (
	"fmt"
	"sync"
	"time"
)

const (
	// breakerBuckets is how many counters a Window is split into; the window slides by one
	// bucket at a time.
	breakerBuckets = 10
	percent        = 100
)

// CircuitBreakerOptions configures Cache.CircuitBreaker and Options.CircuitBreaker. FailurePercent,
// Window and OpenFor are required. Use the same options for the same key.
type CircuitBreakerOptions struct {
	// FailurePercent opens the breaker once this share of calls, from 1 to 99, failed.
	FailurePercent int
	// MinCalls is how many calls the window needs before the breaker may open. 0 means 1.
	MinCalls int
	// Window is the span of recent time calls and failures are counted over.
	Window time.Duration
	// OpenFor is how long the open breaker refuses calls.
	OpenFor time.Duration
	// HalfOpenCalls is how many test calls are let through after OpenFor: the first one that
	// succeeds closes the breaker, one that fails opens it again. 0 means the breaker just
	// closes after OpenFor.
	HalfOpenCalls int
	// IsFailure decides which errors count as failures. nil means any error; for
	// Options.CircuitBreaker, any error but ErrNotFound, ErrWrongType and a done context.
	IsFailure func(error) bool
}

// DoneFunc reports the outcome of a call that CircuitBreaker let through: nil for success, or
// the error the call returned. Call it once.
type DoneFunc func(err error)

// CircuitBreaker checks the breaker for key before a call to a dependency. It returns
// ErrCircuitOpen while the breaker is open; otherwise it returns a DoneFunc to report how the
// call went. The breaker lives in the memory of this process: every process judges the dependency
// by its own calls, and checking it costs no backend round trip.
func (c *Cache) CircuitBreaker(key string, o CircuitBreakerOptions) (DoneFunc, error) {
	if err := o.check(); err != nil {
		return nil, err
	}
	return c.breakers.get(key).allow(o, time.Now())
}

func (o CircuitBreakerOptions) check() error {
	if o.FailurePercent <= 0 || o.FailurePercent >= percent || o.Window <= 0 || o.OpenFor <= 0 ||
		o.MinCalls < 0 || o.HalfOpenCalls < 0 {
		return fmt.Errorf("%w: a breaker needs FailurePercent 1..99, positive Window and OpenFor, "+
			"non-negative MinCalls and HalfOpenCalls", ErrInvalidOptions)
	}
	return nil
}

// breakers holds the state of every breaker key of a Cache.
type breakers struct {
	mu sync.Mutex
	m  map[string]*breaker
}

func (bs *breakers) get(key string) *breaker {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if bs.m == nil {
		bs.m = make(map[string]*breaker)
	}
	b, ok := bs.m[key]
	if !ok {
		b = &breaker{}
		bs.m[key] = b
	}
	return b
}

// breaker is the state of one key: calls and failures per bucket of time, and the open and trial
// phases.
type breaker struct {
	mu         sync.Mutex
	buckets    [breakerBuckets]bucket
	openUntil  time.Time
	trialUntil time.Time // the trial phase lasts until then; zero means none
	trialsLeft int
}

type bucket struct {
	n        int64 // which bucket of time this slot counts; slots are reused around the ring
	total    int
	failures int
}

// breakerState is the phase a breaker is in.
type breakerState int

const (
	// breakerClosed lets every call through and counts failures.
	breakerClosed breakerState = iota
	// breakerOpen refuses every call until OpenFor ends.
	breakerOpen
	// breakerHalfOpen lets HalfOpenCalls test calls through.
	breakerHalfOpen
)

func (b *breaker) state(now time.Time) breakerState {
	switch {
	case now.Before(b.openUntil):
		return breakerOpen
	case now.Before(b.trialUntil):
		return breakerHalfOpen
	}
	return breakerClosed
}

func (b *breaker) allow(o CircuitBreakerOptions, now time.Time) (DoneFunc, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state(now) {
	case breakerOpen:
		return nil, ErrCircuitOpen
	case breakerHalfOpen:
		return b.trial(o)
	case breakerClosed:
	}
	width := max(o.Window/breakerBuckets, 1)
	n := now.UnixNano() / int64(width)
	b.slot(n).total++
	return func(err error) {
		if !failed(o, err) {
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.slot(n).failures++
		if b.overLimit(o, n) {
			b.trip(o, time.Now())
		}
	}, nil
}

// trial lets one test call through while trials are left; its outcome closes or reopens the
// breaker. b.mu is held.
func (b *breaker) trial(o CircuitBreakerOptions) (DoneFunc, error) {
	if b.trialsLeft == 0 {
		return nil, ErrCircuitOpen
	}
	b.trialsLeft--
	return func(err error) {
		b.mu.Lock()
		defer b.mu.Unlock()
		if failed(o, err) {
			b.trip(o, time.Now())
			return
		}
		b.trialUntil = time.Time{} // the dependency works again: closed
	}, nil
}

// slot returns the counters of bucket n, reusing the ring slot of an older bucket.
func (b *breaker) slot(n int64) *bucket {
	s := &b.buckets[n%breakerBuckets]
	if s.n != n {
		*s = bucket{n: n}
	}
	return s
}

// overLimit reports whether the buckets of the window ending at bucket n have enough calls and
// enough failures to open.
func (b *breaker) overLimit(o CircuitBreakerOptions, n int64) bool {
	var total, failures int
	for _, s := range b.buckets {
		if s.n > n-breakerBuckets && s.n <= n {
			total += s.total
			failures += s.failures
		}
	}
	return total >= max(o.MinCalls, 1) && failures*percent >= o.FailurePercent*total
}

// trip opens the breaker, arms the trial phase that follows it and clears the counters.
func (b *breaker) trip(o CircuitBreakerOptions, now time.Time) {
	b.openUntil = now.Add(o.OpenFor)
	b.trialUntil = time.Time{}
	if o.HalfOpenCalls > 0 {
		// Trials are offered for one Window after OpenFor; without them the breaker then closes.
		b.trialUntil = b.openUntil.Add(o.Window)
		b.trialsLeft = o.HalfOpenCalls
	}
	b.buckets = [breakerBuckets]bucket{}
}

func failed(o CircuitBreakerOptions, err error) bool {
	return err != nil && (o.IsFailure == nil || o.IsFailure(err))
}
