// Package nearcache is a two-level gashe.Backend: a small cache in process memory in front of a
// shared backend such as Redis. Hot keys are read from memory, so they do not load one Redis
// node, and reads keep working for a while when the shared backend is slow.
package nearcache

import (
	"context"
	"iter"
	"time"

	"github.com/eugeneliukindev/gashe"
)

const defaultLocalTTL = 5 * time.Second

// Options configures a near cache Backend.
type Options struct {
	// LocalTTL is how long a value read from the shared backend is kept in memory. It is also how
	// long another process may keep answering with a value this process already changed.
	// 0 means 5 seconds.
	LocalTTL time.Duration
}

// Backend reads through local, then shared; writes and deletes go to shared and drop the local
// copy. Locks, counters and TTLs always use shared.
type Backend struct {
	local  gashe.Backend
	shared gashe.Backend
	ttl    time.Duration
}

var _ gashe.Backend = (*Backend)(nil)

// New returns a near cache Backend. local is usually a memory.Backend, shared a Redis backend; both
// are owned by the caller, and Close closes neither. It panics on a negative LocalTTL.
func New(local, shared gashe.Backend, o Options) *Backend {
	if o.LocalTTL < 0 {
		panic("nearcache: negative LocalTTL")
	}
	if o.LocalTTL == 0 {
		o.LocalTTL = defaultLocalTTL
	}
	return &Backend{local: local, shared: shared, ttl: o.LocalTTL}
}

// Get implements gashe.Backend. A value found in shared is copied to local for LocalTTL.
func (b *Backend) Get(ctx context.Context, key string) ([]byte, error) {
	if v, err := b.local.Get(ctx, key); err == nil {
		return v, nil
	}
	v, err := b.shared.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	_ = b.local.Set(ctx, key, v, b.ttl) // a failed local copy only costs the next read
	return v, nil
}

// GetMany implements gashe.Backend. Keys missing in local are read from shared in one call.
func (b *Backend) GetMany(ctx context.Context, keys ...string) ([][]byte, error) {
	out, err := b.local.GetMany(ctx, keys...)
	if err != nil {
		out = make([][]byte, len(keys))
	}
	var missing []string
	var at []int
	for i, v := range out {
		if v == nil {
			missing = append(missing, keys[i])
			at = append(at, i)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	values, err := b.shared.GetMany(ctx, missing...)
	if err != nil {
		return nil, err
	}
	for j, v := range values {
		out[at[j]] = v
		if v != nil {
			_ = b.local.Set(ctx, missing[j], v, b.ttl)
		}
	}
	return out, nil
}

// Set implements gashe.Backend.
func (b *Backend) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	defer b.drop(ctx, key)
	return b.shared.Set(ctx, key, value, ttl)
}

// SetMany implements gashe.Backend.
func (b *Backend) SetMany(ctx context.Context, values map[string][]byte, ttl time.Duration) error {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	defer b.drop(ctx, keys...)
	return b.shared.SetMany(ctx, values, ttl)
}

// SetIfAbsent implements gashe.Backend.
func (b *Backend) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	defer b.drop(ctx, key)
	return b.shared.SetIfAbsent(ctx, key, value, ttl)
}

// SetIfPresent implements gashe.Backend.
func (b *Backend) SetIfPresent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	defer b.drop(ctx, key)
	return b.shared.SetIfPresent(ctx, key, value, ttl)
}

// Delete implements gashe.Backend.
func (b *Backend) Delete(ctx context.Context, key string) (bool, error) {
	defer b.drop(ctx, key)
	return b.shared.Delete(ctx, key)
}

// DeleteMany implements gashe.Backend.
func (b *Backend) DeleteMany(ctx context.Context, keys ...string) error {
	defer b.drop(ctx, keys...)
	return b.shared.DeleteMany(ctx, keys...)
}

// DeleteMatch implements gashe.Backend.
func (b *Backend) DeleteMatch(ctx context.Context, pattern string) error {
	_ = b.local.DeleteMatch(ctx, pattern)
	return b.shared.DeleteMatch(ctx, pattern)
}

// Scan implements gashe.Backend. It scans shared only.
func (b *Backend) Scan(ctx context.Context, pattern string) iter.Seq2[string, error] {
	return b.shared.Scan(ctx, pattern)
}

// Exists implements gashe.Backend. It asks shared, so lock checks see other processes.
func (b *Backend) Exists(ctx context.Context, key string) (bool, error) {
	return b.shared.Exists(ctx, key)
}

// Incr implements gashe.Backend.
func (b *Backend) Incr(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error) {
	defer b.drop(ctx, key)
	return b.shared.Incr(ctx, key, by, ttl)
}

// Expire implements gashe.Backend.
func (b *Backend) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	defer b.drop(ctx, key)
	return b.shared.Expire(ctx, key, ttl)
}

// TTL implements gashe.Backend.
func (b *Backend) TTL(ctx context.Context, key string) (time.Duration, error) {
	return b.shared.TTL(ctx, key)
}

// Unlock implements gashe.Backend.
func (b *Backend) Unlock(ctx context.Context, key string, owner []byte) (bool, error) {
	defer b.drop(ctx, key)
	return b.shared.Unlock(ctx, key, owner)
}

// ExtendLock implements gashe.Backend.
func (b *Backend) ExtendLock(ctx context.Context, key string, owner []byte, ttl time.Duration) (bool, error) {
	return b.shared.ExtendLock(ctx, key, owner, ttl)
}

// Ping implements gashe.Backend. It pings shared.
func (b *Backend) Ping(ctx context.Context) error { return b.shared.Ping(ctx) }

// Close implements gashe.Backend. It does nothing: whoever created the two backends closes them.
func (*Backend) Close() error { return nil }

// drop removes the local copies of keys after a write.
func (b *Backend) drop(ctx context.Context, keys ...string) {
	_ = b.local.DeleteMany(ctx, keys...) // a stale local copy lives at most LocalTTL
}
