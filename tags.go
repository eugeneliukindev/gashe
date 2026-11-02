package gashe

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
)

// maxGeneration bounds a new random generation, leaving room for InvalidateTags to count up.
const maxGeneration = math.MaxInt64 / 2

// InvalidateTags makes every result stored with one of tags a miss. Every tag has a generation
// counter that a result remembers when it is stored; this bumps the counters, so it costs one
// write per tag and no scan of the backend. Tags are shared by every process that uses the same
// backend.
func (c *Cache) InvalidateTags(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		if _, err := c.backend.IncrBy(ctx, ownTag+tag, 1, 0); err != nil {
			return err
		}
	}
	return nil
}

func (l *loading[V]) readGens(ctx context.Context) ([]int64, error) {
	if len(l.o.Tags) == 0 {
		return nil, nil
	}
	keys := l.tagKeys()
	raws, err := l.c.backend.GetMany(ctx, keys...)
	if err == nil {
		var gens []int64
		if gens, err = l.c.generations(ctx, keys, raws); err == nil {
			return gens, nil
		}
	}
	l.c.onError(ctx, fmt.Errorf("gashe: get tags of %q: %w", l.key, err))
	return nil, err
}

func (l *loading[V]) tagKeys() []string {
	keys := make([]string, len(l.o.Tags))
	for i, g := range l.o.Tags {
		keys[i] = ownTag + g
	}
	return keys
}

// generations parses the tag counters read from keys. A missing counter, never created or evicted
// by the backend, gets a random generation: reading it as 0 would make entries invalidated
// before the eviction valid again.
func (c *Cache) generations(ctx context.Context, keys []string, raws [][]byte) ([]int64, error) {
	gens := make([]int64, len(raws))
	for i, raw := range raws {
		if raw == nil {
			gen, err := c.newGeneration(ctx, keys[i])
			if err != nil {
				return nil, err
			}
			gens[i] = gen
			continue
		}
		gens[i], _ = strconv.ParseInt(string(raw), 10, 64)
	}
	return gens, nil
}

// newGeneration creates the missing counter key, or reads the one another caller created first.
func (c *Cache) newGeneration(ctx context.Context, key string) (int64, error) {
	//nolint:gosec // a generation, not a secret
	gen := rand.Int64N(maxGeneration)
	stored, err := c.backend.SetIfAbsent(ctx, key, strconv.AppendInt(nil, gen, 10), 0)
	if err != nil || stored {
		return gen, err
	}
	raw, err := c.backend.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(string(raw), 10, 64)
}
