package gashe

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
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
	done    chan struct{}
	v       any
	err     error
	panic   any                // what fn panicked with, raised again in the caller that started it
	waiters int                // callers still waiting, guarded by group.mu
	cancel  context.CancelFunc // ends the run once no caller waits
	queued  atomic.Bool        // the run waits for a slot of MaxConcurrentLoads
}

// queuedKey is the context key of the call whose run is the context.
type queuedKey struct{}

// markQueued records that the run of ctx waits for a slot, so a caller that leaves meanwhile gets
// ErrOverloaded rather than a bare ctx error.
func markQueued(ctx context.Context, queued bool) {
	if c, ok := ctx.Value(queuedKey{}).(*call); ok {
		c.queued.Store(queued)
	}
}

// do runs fn for k, or waits for the run already in flight. The run has a context of its own,
// with the values of the caller that started it: a caller that leaves, by its ctx, does not fail
// the others, and the run is cancelled only once every caller has left.
func (g *group) do(ctx context.Context, k groupKey, fn func(ctx context.Context) (any, error)) (any, error) {
	g.mu.Lock()
	c, ok := g.calls[k]
	if !ok {
		if g.calls == nil {
			g.calls = make(map[groupKey]*call)
		}
		run, cancel := context.WithCancel(context.WithoutCancel(ctx))
		c = &call{done: make(chan struct{}), err: errLoaderPanicked, cancel: cancel}
		run = context.WithValue(run, queuedKey{}, c)
		g.calls[k] = c
		go g.run(run, k, c, fn)
	}
	c.waiters++
	g.mu.Unlock()

	select {
	case <-c.done:
		if c.panic != nil && !ok {
			panic(c.panic)
		}
		return c.v, c.err
	case <-ctx.Done():
		g.leave(k, c)
		if c.queued.Load() {
			return nil, fmt.Errorf("%w: %w", ErrOverloaded, ctx.Err())
		}
		return nil, ctx.Err()
	}
}

// run calls fn and hands its result to every caller of c.
func (g *group) run(ctx context.Context, k groupKey, c *call, fn func(ctx context.Context) (any, error)) {
	defer func() {
		if p := recover(); p != nil {
			c.panic = p
		}
		c.cancel()
		g.mu.Lock()
		if g.calls[k] == c {
			delete(g.calls, k)
		}
		g.mu.Unlock()
		close(c.done)
	}()
	c.v, c.err = fn(ctx)
}

// leave drops a caller that stopped waiting; the last one cancels the run, and a later caller
// starts a new one.
func (g *group) leave(k groupKey, c *call) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c.waiters--
	if c.waiters == 0 {
		c.cancel()
		if g.calls[k] == c {
			delete(g.calls, k)
		}
	}
}
