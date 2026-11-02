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
	// TTL is how long a result is fresh: a time.Duration, a string for time.ParseDuration such
	// as "1h", or a func(V) time.Duration that picks it from the result, where a negative TTL
	// means "do not store":
	//
	//	TTL: time.Hour
	//	TTL: 90 * time.Second
	//	TTL: "1h30m"
	//	TTL: cfg.UserTTL // a string from the configuration, such as "10m"
	//	TTL: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }
	//	TTL: func(u User) time.Duration {
	//		if u.Admin {
	//			return time.Minute // admins change often
	//		}
	//		return time.Hour
	//	}
	//	TTL: func(p Page) time.Duration {
	//		if p.Draft {
	//			return -1 // do not store drafts
	//		}
	//		return 10 * time.Minute
	//	}
	//
	// Any other type, a plain int such as 3600 included, returns ErrInvalidOptions. TTL may be
	// left out when a window is set: then every call either refreshes in the background
	// (StaleWhileRevalidate) or calls the loader (StaleIfError).
	TTL any
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
	// StaleIfSlow is how long a call waits for the loader while an old result is within
	// StaleIfError. After it, the call answers with the old result, and the load goes on in the
	// background and stores what it gets; meanwhile other calls answer with the old result at
	// once. 0 means wait for the loader. It needs StaleIfError.
	StaleIfSlow time.Duration
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

// check validates o and returns its TTL as a function of the result, so the code after it does
// not care which form TTL was given in.
func (o Policy[V]) check() (ttl func(V) time.Duration, err error) {
	ttl, err = o.ttlFunc()
	if err != nil {
		return nil, err
	}
	for _, rule := range []struct {
		broken bool
		why    string
	}{
		{
			negative(o.StaleWhileRevalidate, o.StaleIfError, o.StaleIfSlow, o.LockOnMiss, o.Negative.TTL),
			"durations must not be negative",
		},
		{o.Jitter < 0 || o.Jitter >= 1, "Jitter must be from 0 to less than 1"},
		{o.Negative.If != nil && (o.Negative.Err == nil || o.Negative.TTL == 0), "Negative needs Err and TTL"},
		{o.StaleIfSlow > 0 && o.StaleIfError == 0, "StaleIfSlow needs StaleIfError"},
	} {
		if rule.broken {
			return nil, fmt.Errorf("%w: %s", ErrInvalidOptions, rule.why)
		}
	}
	return ttl, nil
}

// ttlFunc turns TTL into a function of the result.
func (o Policy[V]) ttlFunc() (func(V) time.Duration, error) {
	switch t := o.TTL.(type) {
	case nil:
		return o.fixedTTL(0)
	case time.Duration:
		return o.fixedTTL(t)
	case string:
		d, err := time.ParseDuration(t)
		if err != nil {
			return nil, fmt.Errorf("%w: TTL: %w", ErrInvalidOptions, err)
		}
		return o.fixedTTL(d)
	case func(V) time.Duration:
		if t == nil {
			return nil, fmt.Errorf("%w: TTL is a nil func", ErrInvalidOptions)
		}
		return t, nil
	}
	return nil, fmt.Errorf("%w: TTL is a %T; want time.Duration, a string such as \"1h\" or func(%v) time.Duration",
		ErrInvalidOptions, o.TTL, reflect.TypeFor[V]())
}

// fixedTTL checks a TTL that does not depend on the result.
func (o Policy[V]) fixedTTL(d time.Duration) (func(V) time.Duration, error) {
	if d < 0 {
		return nil, fmt.Errorf("%w: durations must not be negative", ErrInvalidOptions)
	}
	if d == 0 && o.keep() == 0 {
		return nil, fmt.Errorf("%w: set TTL or a window, or nothing is ever stored", ErrInvalidOptions)
	}
	return func(V) time.Duration { return d }, nil
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
