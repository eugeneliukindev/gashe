package gashe

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// loadKind is a kind of data, as far as MaxConcurrentLoads goes: the type of its results and its
// Policy.Prefix.
type loadKind struct {
	typ    reflect.Type
	prefix string
}

// loadLimits holds the slots of every kind of data a Cache loads with MaxConcurrentLoads.
type loadLimits struct {
	mu sync.Mutex
	m  map[loadKind]chan struct{}
}

// acquire takes a slot of kind, out of n, waiting while all are taken; with now set, a full limit
// fails at once, since an old result can answer. release gives the slot back. n 0 means no limit.
func (ls *loadLimits) acquire(ctx context.Context, kind loadKind, n int, now bool) (release func(), err error) {
	if n == 0 {
		return func() {}, nil
	}
	slots := ls.slots(kind, n)
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	default:
	}
	if now {
		return nil, ErrOverloaded
	}
	markQueued(ctx, true)
	defer markQueued(ctx, false)
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", ErrOverloaded, ctx.Err())
	}
}

// slots returns the slots of kind, made with n on the first load of that kind.
func (ls *loadLimits) slots(kind loadKind, n int) chan struct{} {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.m == nil {
		ls.m = make(map[loadKind]chan struct{})
	}
	s, ok := ls.m[kind]
	if !ok {
		s = make(chan struct{}, n)
		ls.m[kind] = s
	}
	return s
}

// acquire takes a slot of Policy.MaxConcurrentLoads for one load of this kind of data.
func (l *loading[V]) acquire(ctx context.Context) (release func(), err error) {
	return l.c.loads.acquire(
		ctx,
		loadKind{typ: reflect.TypeFor[V](), prefix: l.o.Prefix},
		l.o.MaxConcurrentLoads,
		l.stale,
	)
}
