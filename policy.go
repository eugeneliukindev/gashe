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
// stale: Stale says when it still answers, and the entry is kept for TTL plus its longest window.
type Policy[V any] struct {
	// TTL says how long a result is fresh, and how much that varies.
	TTL TTL
	// Prefix is put in front of every key as is, separator included: "v2:" stores "v2:user:42".
	Prefix string
	// Stale says when a result past its TTL still answers a call: at once while it reloads,
	// instead of a failing loader, or instead of a slow one.
	Stale Stale
	// Stampede says how concurrent calls share loads, so that a key nobody has stored, or many of
	// them, do not send every caller to the source at once.
	Stampede Stampede
	// Tags lists the tags of a result; Cache.InvalidateTags of any of them makes it a miss. Tags
	// are shared by every process that uses the same backend.
	Tags []string
	// Negative caches answers like "not found", so lookups of missing data do not reach the
	// source every time.
	Negative Negative
	// Fallback keeps the results of this kind of data in a backend of its own while the backend of
	// the cache fails or its CircuitBreaker is open, for the TTL but at most Fallback.MaxTTL, and
	// answers from it (ResultFallback): the source gets one load per key and process instead of
	// one per call. The basic API and locks do not use it. Without a Backend there is none: while
	// the backend fails, every call loads.
	Fallback Fallback
	// CacheIf decides whether a result is stored. nil stores every successful result.
	CacheIf func(V) bool
}

// TTL says how long a result is fresh.
type TTL struct {
	// For is how long a result is fresh: a time.Duration, a string for time.ParseDuration such
	// as "1h", or a func(V) time.Duration of the result type that picks it from the result; to
	// leave a result out, use CacheIf, not a negative TTL:
	//
	//	For: time.Hour
	//	For: 90 * time.Second
	//	For: "1h30m"
	//	For: cfg.UserTTL // a string from the configuration, such as "10m"
	//	For: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }
	//	For: func(u User) time.Duration {
	//		if u.Admin {
	//			return time.Minute // admins change often
	//		}
	//		return time.Hour
	//	}
	//
	// Any other type, a plain int such as 3600 included, returns ErrInvalidOptions. For may be
	// left out when a window of Stale is set: then every call either refreshes in the background
	// (Stale.WhileRevalidate) or calls the loader (Stale.IfError, Stale.IfSlow).
	For any
	// Jitter spreads TTLs randomly by up to this share, from 0 to 1 (0.1 means +-10%), so that
	// entries stored together do not expire together.
	Jitter float64
}

// Stampede says how concurrent calls share loads: within this process with SingleFlight, across
// processes with LockFor, and how many loads of one kind run at once with MaxLoads. They work
// together: with SingleFlight the calls of a process share one load first, and only that load
// takes the lock of LockFor.
type Stampede struct {
	// SingleFlight makes concurrent calls for one key and type in this process share one lookup and
	// one load. The load runs with the ctx values of the call that started it: leave it off when
	// the loader reads from ctx what the key does not name, such as the user of the request.
	SingleFlight bool
	// LockFor, when set, makes a missing key load under a lock: one process calls the loader, the
	// others wait and take its result. The lock is shared by every process that uses the same
	// backend, and LockFor is the longest it is held, so a process that dies holding it frees it.
	LockFor time.Duration
	// MaxLoads is how many loads of this kind of data, its type and Prefix, may run at once in
	// this process: with 10 processes, up to 10 times as many reach the source. A call over the
	// limit waits for a slot as long as its ctx allows, then returns ErrOverloaded; one that the
	// stale result can answer, within Stale.IfError or Stale.IfSlow or a background reload, does
	// not wait. The wait has no limit but ctx: give calls a deadline, or calls over the limit pile
	// up. Use the same value for the same kind of data. 0 means no limit.
	MaxLoads int
}

// Negative caches loader errors that mean "there is no such data".
type Negative struct {
	// IsNotFound picks the loader errors that mean "not found": answers, not failures. GetOrLoad returns
	// ErrNotFound for them, wrapping the loader error when it loaded, and alone while the answer
	// is cached; the stale result never hides them. nil turns negative caching off.
	IsNotFound func(error) bool
	// TTL is how long the negative answer is kept.
	TTL time.Duration
}

// Stale says when a stored result past its TTL, the stale result, still answers a call. Each way
// has its own window after TTL; the zero value of each is off.
type Stale struct {
	// WhileRevalidate is how long after TTL the stale result answers at once, while one caller
	// reloads it in the background (stale-while-revalidate of RFC 5861). The refresh lock is
	// shared by every process that uses the same backend.
	WhileRevalidate time.Duration
	// IfError answers a failing loader with the stale result.
	IfError StaleIfError
	// IfSlow answers a slow loader with the stale result.
	IfSlow StaleIfSlow
}

// StaleIfError answers a loader error with the stale result (stale-if-error of RFC 5861), so
// callers keep getting data while the source is down.
type StaleIfError struct {
	// For is how long after TTL the stale result answers a failure. 0 turns it off.
	For time.Duration
	// IsFailure picks the loader errors that are failures. nil means any error. An error
	// Negative.IsNotFound accepts is never one: it is the answer "not found".
	IsFailure func(error) bool
}

// StaleIfSlow answers a slow loader with the stale result: the call waits at most After, then gets
// the stale result while the load goes on in the background and stores what it gets; meanwhile
// other calls of the key get the stale result at once.
type StaleIfSlow struct {
	// For is how long after TTL the stale result answers a slow loader. 0 turns it off.
	For time.Duration
	// After is how long a call waits for the loader before the stale result answers it.
	After time.Duration
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
			negative(o.Stale.WhileRevalidate, o.Stale.IfError.For, o.Stale.IfSlow.For, o.Stale.IfSlow.After,
				o.Stampede.LockFor, o.Negative.TTL),
			"durations must not be negative",
		},
		{o.TTL.Jitter < 0 || o.TTL.Jitter >= 1, "TTL.Jitter must be from 0 to less than 1"},
		{o.Negative.IsNotFound != nil && o.Negative.TTL == 0, "Negative needs a TTL"},
		{(o.Stale.IfSlow.For > 0) != (o.Stale.IfSlow.After > 0), "Stale.IfSlow needs both For and After"},
		{o.Stampede.MaxLoads < 0, "Stampede.MaxLoads must not be negative"},
		{
			o.Fallback.MaxTTL < 0 || (o.Fallback.Backend == nil && o.Fallback.MaxTTL != 0),
			"Fallback needs a Backend and a MaxTTL that is not negative",
		},
	} {
		if rule.broken {
			return nil, fmt.Errorf("%w: %s", ErrInvalidOptions, rule.why)
		}
	}
	return ttl, nil
}

// ttlFunc turns TTL into a function of the result.
func (o Policy[V]) ttlFunc() (func(V) time.Duration, error) {
	switch t := o.TTL.For.(type) {
	case nil:
		return o.fixedTTL(0)
	case time.Duration:
		return o.fixedTTL(t)
	case string:
		d, err := time.ParseDuration(t)
		if err != nil {
			return nil, fmt.Errorf("%w: TTL.For: %w", ErrInvalidOptions, err)
		}
		return o.fixedTTL(d)
	case func(V) time.Duration:
		if t == nil {
			return nil, fmt.Errorf("%w: TTL.For is a nil func", ErrInvalidOptions)
		}
		return t, nil
	}
	return nil, fmt.Errorf("%w: TTL.For is a %T; want time.Duration, a string such as \"1h\" or func(%v) time.Duration",
		ErrInvalidOptions, o.TTL.For, reflect.TypeFor[V]())
}

// fixedTTL checks a TTL that does not depend on the result.
func (o Policy[V]) fixedTTL(d time.Duration) (func(V) time.Duration, error) {
	if d < 0 {
		return nil, fmt.Errorf("%w: durations must not be negative", ErrInvalidOptions)
	}
	if d == 0 && o.keep() == 0 {
		return nil, fmt.Errorf("%w: set TTL.For or a window, or nothing is ever stored", ErrInvalidOptions)
	}
	return func(V) time.Duration { return d }, nil
}

func negative(ds ...time.Duration) bool {
	return slices.ContainsFunc(ds, func(d time.Duration) bool { return d < 0 })
}

// keep is how long an entry outlives its fresh period.
func (o Policy[V]) keep() time.Duration {
	return max(o.Stale.WhileRevalidate, o.Stale.IfError.For, o.Stale.IfSlow.For)
}

// spread applies Jitter to d.
func (o Policy[V]) spread(d time.Duration) time.Duration {
	if o.TTL.Jitter == 0 || d <= 0 {
		return d
	}
	return time.Duration(float64(d) * (1 + (rand.Float64()*2-1)*o.TTL.Jitter)) //nolint:gosec // jitter, not a secret
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
