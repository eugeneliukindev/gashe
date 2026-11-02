package gashe

import (
	"context"
	"errors"
	"reflect"
	"sync"
)

var errLoaderPanicked = errors.New("gashe: loader panicked")

// groupKey names one call: the same key loaded as different types are different calls.
type groupKey struct {
	key string
	typ reflect.Type
}

// group runs one call per key and type at a time inside the process; concurrent callers share
// its result.
type group struct {
	mu    sync.Mutex
	calls map[groupKey]*call
}

type call struct {
	done chan struct{}
	v    any
	err  error
}

// do runs fn for k, or waits for the run already in flight. The run uses the context of the
// caller that started it; a waiter stops waiting when its own ctx is done.
func (g *group) do(ctx context.Context, k groupKey, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if c, ok := g.calls[k]; ok {
		g.mu.Unlock()
		select {
		case <-c.done:
			return c.v, c.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if g.calls == nil {
		g.calls = make(map[groupKey]*call)
	}
	c := &call{done: make(chan struct{}), err: errLoaderPanicked}
	g.calls[k] = c
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.calls, k)
		g.mu.Unlock()
		close(c.done)
	}()
	c.v, c.err = fn()
	return c.v, c.err
}
