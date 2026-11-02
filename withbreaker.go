package gashe

import (
	"context"
	"errors"
	"iter"
	"time"
)

// WithBreaker returns a Backend that stops calling b for a while once too many calls to it fail,
// as CircuitBreaker does for a dependency. While it is open every call returns ErrCircuitOpen at
// once, so a cache that is down costs no timeout: GetOrLoad goes straight to the loader. The
// breaker lives in the memory of this process. Failures are errors other than ErrNotFound,
// ErrWrongType and a done context, unless o.IsFailure says otherwise. Ping and Close are not
// guarded, so health checks see the backend itself. It panics on invalid options.
func WithBreaker(b Backend, o BreakerOptions) Backend {
	if err := o.check(); err != nil {
		panic(err)
	}
	if o.IsFailure == nil {
		o.IsFailure = backendFailed
	}
	return breakerBackend{Backend: b, breaker: &breaker{}, o: o}
}

// backendFailed reports whether err means the backend is unwell rather than a normal answer.
func backendFailed(err error) bool {
	return !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrWrongType) &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

type breakerBackend struct {
	Backend

	breaker *breaker
	o       BreakerOptions
}

// guard runs call through the breaker.
func guard[T any](p breakerBackend, call func() (T, error)) (T, error) {
	done, err := p.breaker.allow(p.o, time.Now())
	if err != nil {
		var zero T
		return zero, err
	}
	v, err := call()
	done(err)
	return v, err
}

// guardErr runs a call that returns only an error through the breaker.
func guardErr(p breakerBackend, call func() error) error {
	_, err := guard(p, func() (struct{}, error) { return struct{}{}, call() })
	return err
}

func (p breakerBackend) Get(ctx context.Context, key string) ([]byte, error) {
	return guard(p, func() ([]byte, error) { return p.Backend.Get(ctx, key) })
}

func (p breakerBackend) GetMany(ctx context.Context, keys ...string) ([][]byte, error) {
	return guard(p, func() ([][]byte, error) { return p.Backend.GetMany(ctx, keys...) })
}

func (p breakerBackend) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return guardErr(p, func() error { return p.Backend.Set(ctx, key, value, ttl) })
}

func (p breakerBackend) SetMany(ctx context.Context, values map[string][]byte, ttl time.Duration) error {
	return guardErr(p, func() error { return p.Backend.SetMany(ctx, values, ttl) })
}

func (p breakerBackend) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return guard(p, func() (bool, error) { return p.Backend.SetIfAbsent(ctx, key, value, ttl) })
}

func (p breakerBackend) SetIfPresent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return guard(p, func() (bool, error) { return p.Backend.SetIfPresent(ctx, key, value, ttl) })
}

func (p breakerBackend) Delete(ctx context.Context, key string) (bool, error) {
	return guard(p, func() (bool, error) { return p.Backend.Delete(ctx, key) })
}

func (p breakerBackend) DeleteMany(ctx context.Context, keys ...string) error {
	return guardErr(p, func() error { return p.Backend.DeleteMany(ctx, keys...) })
}

func (p breakerBackend) DeleteMatch(ctx context.Context, pattern string) error {
	return guardErr(p, func() error { return p.Backend.DeleteMatch(ctx, pattern) })
}

func (p breakerBackend) Scan(ctx context.Context, pattern string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		done, err := p.breaker.allow(p.o, time.Now())
		if err != nil {
			yield("", err)
			return
		}
		var scanErr error
		for key, err := range p.Backend.Scan(ctx, pattern) {
			if err != nil {
				scanErr = err
			}
			if !yield(key, err) {
				break
			}
		}
		done(scanErr)
	}
}

func (p breakerBackend) Exists(ctx context.Context, key string) (bool, error) {
	return guard(p, func() (bool, error) { return p.Backend.Exists(ctx, key) })
}

func (p breakerBackend) Incr(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error) {
	return guard(p, func() (int64, error) { return p.Backend.Incr(ctx, key, by, ttl) })
}

func (p breakerBackend) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return guard(p, func() (bool, error) { return p.Backend.Expire(ctx, key, ttl) })
}

func (p breakerBackend) TTL(ctx context.Context, key string) (time.Duration, error) {
	return guard(p, func() (time.Duration, error) { return p.Backend.TTL(ctx, key) })
}

func (p breakerBackend) Unlock(ctx context.Context, key string, owner []byte) (bool, error) {
	return guard(p, func() (bool, error) { return p.Backend.Unlock(ctx, key, owner) })
}

func (p breakerBackend) ExtendLock(ctx context.Context, key string, owner []byte, ttl time.Duration) (bool, error) {
	return guard(p, func() (bool, error) { return p.Backend.ExtendLock(ctx, key, owner, ttl) })
}
