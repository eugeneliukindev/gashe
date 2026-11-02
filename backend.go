package gashe

import (
	"context"
	"iter"
	"time"
)

// NoExpiry is what Backend.TTL returns for an entry that never expires.
const NoExpiry time.Duration = -1

// Backend is a place where entries live. It stores bytes only; Cache turns values into bytes.
//
// A ttl of 0 means "no expiry" unless a method says otherwise. Methods that read a single
// entry return ErrNotFound on a miss. Patterns use Redis glob syntax: *, ?, [abc], [^a], [a-z]
// and \ to escape. All methods must be safe for concurrent use.
type Backend interface {
	// Get returns the value stored under key.
	Get(ctx context.Context, key string) ([]byte, error)
	// GetMany returns values in the order of keys; a missing key gives a nil item.
	GetMany(ctx context.Context, keys ...string) ([][]byte, error)
	// Set stores value under key, replacing the entry and its TTL.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// SetMany stores all values with the same TTL.
	SetMany(ctx context.Context, values map[string][]byte, ttl time.Duration) error
	// SetIfAbsent stores value only if key has no live entry and reports whether it did.
	SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (stored bool, err error)
	// SetIfPresent stores value only if key has a live entry and reports whether it did.
	SetIfPresent(ctx context.Context, key string, value []byte, ttl time.Duration) (stored bool, err error)
	// Delete removes key and reports whether it existed.
	Delete(ctx context.Context, key string) (existed bool, err error)
	// DeleteMany removes all keys.
	DeleteMany(ctx context.Context, keys ...string) error
	// DeleteMatch removes every key that matches pattern.
	DeleteMatch(ctx context.Context, pattern string) error
	// Scan yields every key that matches pattern. A failure is yielded once as the error, and
	// the scan stops.
	Scan(ctx context.Context, pattern string) iter.Seq2[string, error]
	// Exists reports whether key has a live entry.
	Exists(ctx context.Context, key string) (bool, error)
	// IncrBy adds by to the integer under key, creating it from 0 if absent, and returns the result.
	// ttl applies only when the entry is created.
	IncrBy(ctx context.Context, key string, by int64, ttl time.Duration) (value int64, err error)
	// Expire sets a new TTL (ttl > 0) for key and reports whether the key existed.
	Expire(ctx context.Context, key string, ttl time.Duration) (existed bool, err error)
	// TTL returns the time key has left, or NoExpiry.
	TTL(ctx context.Context, key string) (left time.Duration, err error)
	// Unlock removes key only if it holds owner and reports whether it did.
	Unlock(ctx context.Context, key string, owner []byte) (released bool, err error)
	// ExtendLock sets a new TTL (ttl > 0) for key only if it holds owner and reports whether it did.
	ExtendLock(ctx context.Context, key string, owner []byte, ttl time.Duration) (extended bool, err error)
	// Ping checks that the backend is reachable.
	Ping(ctx context.Context) error
	// Close releases resources the backend owns.
	Close() error
}
