package gashe

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"time"
)

// Policy says how GetOrLoad caches results of type V. Define it once and pass it to
// every call; the zero value of each field means "off".
//
// A stored result is fresh for TTL: it is answered without calling the loader. After TTL it is
// old: StaleWhileRevalidate and StaleIfError say for how much longer it is still good enough. The
// entry is kept for TTL plus the longer of the two windows.
type Policy[V any] struct {
	// TTL is how long a result is fresh. It may be 0 when a window is set: then every call
	// either refreshes in the background (StaleWhileRevalidate) or calls the loader (StaleIfError).
	TTL time.Duration
	// TTLFunc, when set, picks TTL from the result. A negative TTL means "do not store".
	TTLFunc func(V) time.Duration
	// Jitter spreads TTLs randomly by up to this share, from 0 to 1 (0.1 means +-10%), so that
	// entries stored together do not expire together.
	Jitter float64
	// StaleWhileRevalidate is how long after TTL an old result is still answered at once, while one
	// caller reloads it in the background. The refresh lock is shared by every process that uses
	// the same backend.
	StaleWhileRevalidate time.Duration
	// StaleIfError is how long after TTL an old result answers a loader error, so callers keep
	// getting data while the source is down.
	StaleIfError time.Duration
	// StaleIf decides which loader errors StaleIfError covers. nil means any error.
	StaleIf func(error) bool
	// LockOnMiss, when set, makes a missing key load under a lock: one process calls the loader,
	// the others wait and take its result. The value is the longest the lock is held. The lock is
	// shared by every process that uses the same backend.
	LockOnMiss time.Duration
	// Negative caches answers like "not found", so lookups of missing data do not reach the
	// source every time.
	Negative Negative
	// Tags lists the tags of a result; Cache.InvalidateTags of any of them makes it a miss. Tags
	// are shared by every process that uses the same backend.
	Tags []string
	// CacheIf decides whether a result is stored. nil stores every successful result.
	CacheIf func(V) bool
	// Prefix is put in front of every key as is, separator included: "v2:" stores "v2:user:42".
	Prefix string
}

// Negative caches loader errors that mean "there is no such data".
type Negative struct {
	// If picks the loader errors to cache. nil turns negative caching off.
	If func(error) bool
	// Err is the error GetOrLoad returns while the negative answer is cached. Return a sentinel
	// your code already checks, such as sql.ErrNoRows.
	Err error
	// TTL is how long the negative answer is kept.
	TTL time.Duration
}

func (o Policy[V]) check() error {
	for _, rule := range []struct {
		broken bool
		why    string
	}{
		{
			negative(o.TTL, o.StaleWhileRevalidate, o.StaleIfError, o.LockOnMiss, o.Negative.TTL),
			"durations must not be negative",
		},
		{o.Jitter < 0 || o.Jitter >= 1, "Jitter must be from 0 to less than 1"},
		{o.TTL == 0 && o.TTLFunc == nil && o.keep() == 0, "set TTL, TTLFunc or a window, or nothing is ever stored"},
		{o.Negative.If != nil && (o.Negative.Err == nil || o.Negative.TTL == 0), "Negative needs Err and TTL"},
	} {
		if rule.broken {
			return fmt.Errorf("%w: %s", ErrInvalidOptions, rule.why)
		}
	}
	return nil
}

func negative(ds ...time.Duration) bool {
	return slices.ContainsFunc(ds, func(d time.Duration) bool { return d < 0 })
}

// keep is how long an entry outlives its fresh period.
func (o Policy[V]) keep() time.Duration { return max(o.StaleWhileRevalidate, o.StaleIfError) }

// spread applies Jitter to d.
func (o Policy[V]) spread(d time.Duration) time.Duration {
	if o.Jitter == 0 || d <= 0 {
		return d
	}
	return time.Duration(float64(d) * (1 + (rand.Float64()*2-1)*o.Jitter)) //nolint:gosec // jitter, not a secret
}

// NotNil is a CacheIf rule that skips nil pointers, slices, maps, channels, functions and interfaces.
// Values of other kinds always pass.
func NotNil[V any](v V) bool {
	rv := reflect.ValueOf(any(v))
	if !rv.IsValid() {
		return false
	}
	switch rv.Kind() { //nolint:exhaustive // only nillable kinds matter
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		return !rv.IsNil()
	default:
		return true
	}
}
