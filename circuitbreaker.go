package gashe

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	breakerPrefix = "circuit_breaker:"
	// breakerBuckets is how many counters a Window is split into; the window slides by one
	// bucket at a time.
	breakerBuckets = 10
	// countersPerBucket is the total and the failures counter of a bucket.
	countersPerBucket = 2
	percent           = 100

	openSuffix     = ":open"
	trialSuffix    = ":trial"
	trialsSuffix   = ":trials"
	totalSuffix    = ":total:"
	failuresSuffix = ":failures:"
)

// BreakerOptions configures Cache.CircuitBreaker. FailurePercent, Window and OpenFor are
// required.
type BreakerOptions struct {
	// FailurePercent opens the breaker once this share of calls, from 1 to 99, failed.
	FailurePercent int
	// MinCalls is how many calls the window needs before the breaker may open. 0 means 1.
	MinCalls int
	// Window is the span of recent time calls and failures are counted over.
	Window time.Duration
	// OpenFor is how long the open breaker refuses calls.
	OpenFor time.Duration
	// TrialCalls is how many test calls are let through after OpenFor: the first one that
	// succeeds closes the breaker, one that fails opens it again. 0 means the breaker just
	// closes after OpenFor.
	TrialCalls int
	// IsFailure decides which errors count as failures. nil means any error.
	IsFailure func(error) bool
}

// DoneFunc reports the outcome of a call that CircuitBreaker let through: nil for success, or
// the error the call returned. Call it exactly once.
type DoneFunc func(err error)

// CircuitBreaker checks the breaker for key before a call to a dependency. It returns
// ErrCircuitOpen while the breaker is open; otherwise it returns a DoneFunc to report how the
// call went. The breaker is shared by every process that uses the backend. If the backend fails,
// the breaker counts as closed and the error goes to Options.OnError.
func (c *Cache) CircuitBreaker(ctx context.Context, key string, o BreakerOptions) (DoneFunc, error) {
	if err := o.check(); err != nil {
		return nil, err
	}
	b := &breaker{c: c, o: o, key: breakerPrefix + key, bucket: max(o.Window/breakerBuckets, 1)}
	if b.exists(ctx, b.key+openSuffix) {
		return nil, ErrCircuitOpen
	}
	bg := context.WithoutCancel(ctx)
	if b.exists(ctx, b.key+trialSuffix) {
		return b.trial(ctx, bg)
	}
	now := b.bucketOf(time.Now())
	b.incr(ctx, b.key+totalSuffix+now, o.Window+b.bucket)
	return func(err error) {
		if !b.failed(err) {
			return
		}
		b.incr(bg, b.key+failuresSuffix+now, o.Window+b.bucket)
		if b.overLimit(bg) {
			b.trip(bg)
		}
	}, nil
}

func (o BreakerOptions) check() error {
	if o.FailurePercent <= 0 || o.FailurePercent >= percent || o.Window <= 0 || o.OpenFor <= 0 ||
		o.MinCalls < 0 || o.TrialCalls < 0 {
		return fmt.Errorf(
			"%w: CircuitBreaker needs FailurePercent 1..99, positive Window and OpenFor, "+
				"non-negative MinCalls and TrialCalls", ErrInvalidOptions)
	}
	return nil
}

// breaker is one CircuitBreaker check of one key.
type breaker struct {
	c      *Cache
	o      BreakerOptions
	key    string
	bucket time.Duration
}

// trial lets the call through as a trial if a trial slot is left: its outcome closes the
// breaker or opens it again.
func (b *breaker) trial(ctx, bg context.Context) (DoneFunc, error) {
	if b.incr(ctx, b.key+trialsSuffix, b.o.Window) > int64(b.o.TrialCalls) {
		return nil, ErrCircuitOpen
	}
	return func(err error) {
		if b.failed(err) {
			b.trip(bg)
			return
		}
		b.close(bg)
	}, nil
}

func (b *breaker) failed(err error) bool {
	return err != nil && (b.o.IsFailure == nil || b.o.IsFailure(err))
}

// overLimit reports whether the window has enough calls and enough failures to open.
func (b *breaker) overLimit(ctx context.Context) bool {
	raws, err := b.c.backend.GetMany(ctx, b.bucketKeys()...)
	if err != nil {
		b.report(ctx, err)
		return false
	}
	var total, failures int64
	for i, raw := range raws {
		n, _ := strconv.ParseInt(string(raw), 10, 64) // a missing bucket reads as 0
		if i%countersPerBucket == 0 {
			total += n
		} else {
			failures += n
		}
	}
	return total >= int64(max(b.o.MinCalls, 1)) && failures*percent >= int64(b.o.FailurePercent)*total
}

// trip opens the breaker, arms the trial phase that follows it and clears the counters.
func (b *breaker) trip(ctx context.Context) {
	if err := b.c.backend.Set(ctx, b.key+openSuffix, []byte("1"), b.o.OpenFor); err != nil {
		b.report(ctx, err)
	}
	if b.o.TrialCalls > 0 {
		// Trials are offered for one Window after OpenFor; without them the breaker then closes.
		if err := b.c.backend.Set(ctx, b.key+trialSuffix, []byte("1"), b.o.OpenFor+b.o.Window); err != nil {
			b.report(ctx, err)
		}
	}
	b.deleteAll(ctx, b.bucketKeys()...)
}

// close closes the breaker after a successful trial.
func (b *breaker) close(ctx context.Context) {
	b.deleteAll(ctx, append(b.bucketKeys(), b.key+trialSuffix)...)
}

func (b *breaker) deleteAll(ctx context.Context, keys ...string) {
	if err := b.c.backend.DeleteMany(ctx, append(keys, b.key+trialsSuffix)...); err != nil {
		b.report(ctx, err)
	}
}

// bucketKeys returns the total and failures counters of the buckets the window covers,
// in pairs: total, failures, total, failures, ...
func (b *breaker) bucketKeys() []string {
	now := time.Now()
	keys := make([]string, 0, countersPerBucket*breakerBuckets)
	for i := range breakerBuckets {
		n := b.bucketOf(now.Add(-time.Duration(i) * b.bucket))
		keys = append(keys, b.key+totalSuffix+n, b.key+failuresSuffix+n)
	}
	return keys
}

func (b *breaker) bucketOf(t time.Time) string {
	return strconv.FormatInt(t.UnixNano()/int64(b.bucket), 10)
}

// incr adds one to the counter under key, created with ttl, and returns the result, or 0 if
// the backend fails.
func (b *breaker) incr(ctx context.Context, key string, ttl time.Duration) int64 {
	n, err := b.c.backend.Incr(ctx, key, 1, ttl)
	if err != nil {
		b.report(ctx, err)
	}
	return n
}

func (b *breaker) exists(ctx context.Context, key string) bool {
	ok, err := b.c.backend.Exists(ctx, key)
	if err != nil {
		b.report(ctx, err)
	}
	return ok
}

func (b *breaker) report(ctx context.Context, err error) {
	b.c.onError(ctx, fmt.Errorf("gashe: circuit breaker %q: %w", b.key, err))
}
