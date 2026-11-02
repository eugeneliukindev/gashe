package gashe

import (
	"context"
	"fmt"
	"strconv"
)

// tagPrefix is where the generation counter of every tag is stored.
const tagPrefix = "gashe:tag:"

// InvalidateTags makes every result stored with one of tags a miss. Every tag has a generation
// counter that a result remembers when it is stored; this bumps the counters, so it costs one
// write per tag and no scan of the backend. Tags are shared by every process that uses the same
// backend.
func (c *Cache) InvalidateTags(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		if _, err := c.backend.Incr(ctx, tagPrefix+tag, 1, 0); err != nil {
			return err
		}
	}
	return nil
}

func (l *loading[V]) readGens(ctx context.Context) ([]int64, error) {
	if len(l.o.Tags) == 0 {
		return nil, nil
	}
	raws, err := l.c.backend.GetMany(ctx, l.tagKeys()...)
	if err != nil {
		l.c.onError(ctx, fmt.Errorf("gashe: get tags of %q: %w", l.key, err))
		return nil, err
	}
	return generations(raws), nil
}

func (l *loading[V]) tagKeys() []string {
	keys := make([]string, len(l.o.Tags))
	for i, g := range l.o.Tags {
		keys[i] = tagPrefix + g
	}
	return keys
}

// generations parses tag counters; a missing counter is generation 0.
func generations(raws [][]byte) []int64 {
	gens := make([]int64, len(raws))
	for i, raw := range raws {
		gens[i], _ = strconv.ParseInt(string(raw), 10, 64)
	}
	return gens
}
