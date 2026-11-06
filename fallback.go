package gashe

import (
	"cmp"
	"context"
	"fmt"
	"time"
)

// defaultFallbackTTL is the MaxTTL of a fallback that sets none.
const defaultFallbackTTL = 30 * time.Second

// FallbackOptions configures Policy.Fallback: where loads keep their results while the backend of
// the cache fails, so the source gets one load per key and process, not one per call.
type FallbackOptions struct {
	// Backend keeps the results, usually a memory.Backend of its own; required. Policies of one
	// Cache may share it; a near cache or another Cache may not.
	Backend Backend
	// MaxTTL is the longest a result stays in Backend: the TTL of its policy, but never longer, since
	// no invalidation reaches Backend while the backend is down. 0 means 30 seconds.
	MaxTTL time.Duration
}

// spare keeps results in Policy.Fallback.Backend while the backend fails; without a Fallback it
// keeps nothing.
type spare struct {
	backend Backend // nil without a Fallback
	maxTTL  time.Duration
	onError func(ctx context.Context, err error)
}

// spareOf returns the spare of a policy with fallback f, already checked by Policy.check.
func (c *Cache) spareOf(f *FallbackOptions) spare {
	if f == nil {
		return spare{}
	}
	return spare{backend: f.Backend, maxTTL: cmp.Or(f.MaxTTL, defaultFallbackTTL), onError: c.onError}
}

func (s spare) get(ctx context.Context, key string) ([]byte, bool) {
	if s.backend == nil {
		return nil, false
	}
	raw, err := s.backend.Get(ctx, key)
	return raw, err == nil
}

// put keeps raw for ttl, but never longer than MaxTTL; a result never fresh keeps MaxTTL.
func (s spare) put(ctx context.Context, key string, raw []byte, ttl time.Duration) {
	if s.backend == nil {
		return
	}
	if ttl <= 0 || ttl > s.maxTTL {
		ttl = s.maxTTL
	}
	if err := s.backend.Set(ctx, key, raw, ttl); err != nil {
		s.onError(ctx, fmt.Errorf("gashe: fallback set %q: %w", key, err))
	}
}
