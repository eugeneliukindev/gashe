package gashe

import (
	"context"
	"fmt"
	"time"
)

// defaultFallbackTTL is the MaxTTL of a fallback that sets none.
const defaultFallbackTTL = 30 * time.Second

// FallbackOptions configures Options.Fallback: where GetOrLoad keeps results while the backend of
// the cache fails, so the source gets one load per key and process, not one per call.
type FallbackOptions struct {
	// Backend keeps the results, usually a memory.Backend of its own; required. Do not share it with
	// a near cache or another Cache.
	Backend Backend
	// MaxTTL is the longest a result stays in Backend: the TTL of its policy, but never longer, since
	// no invalidation reaches Backend while the backend is down. 0 means 30 seconds.
	MaxTTL time.Duration
}

// spare keeps results while the backend fails.
type spare interface {
	get(ctx context.Context, key string) ([]byte, bool)
	put(ctx context.Context, key string, raw []byte, ttl time.Duration)
}

// noSpare is the spare of a Cache without Options.Fallback: it keeps nothing.
type noSpare struct{}

func (noSpare) get(context.Context, string) ([]byte, bool)         { return nil, false }
func (noSpare) put(context.Context, string, []byte, time.Duration) {}

// backendSpare keeps results in Options.Fallback.Backend.
type backendSpare struct {
	backend Backend
	maxTTL  time.Duration
	onError func(ctx context.Context, err error)
}

func newSpare(o *FallbackOptions, onError func(ctx context.Context, err error)) backendSpare {
	if o.Backend == nil {
		panic("gashe: Fallback without a Backend")
	}
	if o.MaxTTL < 0 {
		panic("gashe: negative Fallback.MaxTTL")
	}
	maxTTL := o.MaxTTL
	if maxTTL == 0 {
		maxTTL = defaultFallbackTTL
	}
	return backendSpare{backend: o.Backend, maxTTL: maxTTL, onError: onError}
}

func (s backendSpare) get(ctx context.Context, key string) ([]byte, bool) {
	raw, err := s.backend.Get(ctx, key)
	return raw, err == nil
}

// put keeps raw for ttl, but never longer than MaxTTL; a result never fresh keeps MaxTTL.
func (s backendSpare) put(ctx context.Context, key string, raw []byte, ttl time.Duration) {
	if ttl <= 0 || ttl > s.maxTTL {
		ttl = s.maxTTL
	}
	if err := s.backend.Set(ctx, key, raw, ttl); err != nil {
		s.onError(ctx, fmt.Errorf("gashe: fallback set %q: %w", key, err))
	}
}
