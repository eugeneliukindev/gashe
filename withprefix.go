package gashe

import (
	"context"
	"iter"
	"strings"
	"time"
)

// withPrefix returns a Backend that keeps every key of b under prefix; Options.Prefix uses it.
// Scan yields keys without the prefix, and the prefix is escaped in patterns.
func withPrefix(b Backend, prefix string) Backend {
	escape := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `]`, `\]`)
	return prefixBackend{Backend: b, prefix: prefix, pattern: escape.Replace(prefix)}
}

type prefixBackend struct {
	Backend

	prefix  string
	pattern string // prefix with glob characters escaped
}

func (p prefixBackend) Get(ctx context.Context, key string) ([]byte, error) {
	return p.Backend.Get(ctx, p.prefix+key)
}

func (p prefixBackend) GetMany(ctx context.Context, keys ...string) ([][]byte, error) {
	return p.Backend.GetMany(ctx, p.keys(keys)...)
}

func (p prefixBackend) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return p.Backend.Set(ctx, p.prefix+key, value, ttl)
}

func (p prefixBackend) SetMany(ctx context.Context, values map[string][]byte, ttl time.Duration) error {
	out := make(map[string][]byte, len(values))
	for k, v := range values {
		out[p.prefix+k] = v
	}
	return p.Backend.SetMany(ctx, out, ttl)
}

func (p prefixBackend) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return p.Backend.SetIfAbsent(ctx, p.prefix+key, value, ttl)
}

func (p prefixBackend) SetIfPresent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return p.Backend.SetIfPresent(ctx, p.prefix+key, value, ttl)
}

func (p prefixBackend) Delete(ctx context.Context, key string) (bool, error) {
	return p.Backend.Delete(ctx, p.prefix+key)
}

func (p prefixBackend) DeleteMany(ctx context.Context, keys ...string) error {
	return p.Backend.DeleteMany(ctx, p.keys(keys)...)
}

func (p prefixBackend) DeleteMatch(ctx context.Context, pattern string) error {
	return p.Backend.DeleteMatch(ctx, p.pattern+pattern)
}

func (p prefixBackend) Scan(ctx context.Context, pattern string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for key, err := range p.Backend.Scan(ctx, p.pattern+pattern) {
			if !yield(strings.TrimPrefix(key, p.prefix), err) {
				return
			}
		}
	}
}

func (p prefixBackend) Exists(ctx context.Context, key string) (bool, error) {
	return p.Backend.Exists(ctx, p.prefix+key)
}

func (p prefixBackend) IncrBy(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error) {
	return p.Backend.IncrBy(ctx, p.prefix+key, by, ttl)
}

func (p prefixBackend) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return p.Backend.Expire(ctx, p.prefix+key, ttl)
}

func (p prefixBackend) TTL(ctx context.Context, key string) (time.Duration, error) {
	return p.Backend.TTL(ctx, p.prefix+key)
}

func (p prefixBackend) Unlock(ctx context.Context, key string, owner []byte) (bool, error) {
	return p.Backend.Unlock(ctx, p.prefix+key, owner)
}

func (p prefixBackend) ExtendLock(ctx context.Context, key string, owner []byte, ttl time.Duration) (bool, error) {
	return p.Backend.ExtendLock(ctx, p.prefix+key, owner, ttl)
}

func (p prefixBackend) keys(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = p.prefix + k
	}
	return out
}
