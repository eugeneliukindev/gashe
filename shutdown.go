package gashe

import (
	"context"
	"sync"
)

// Shutdown stops starting background reloads and waits for the running ones to finish. If ctx is
// done first, it returns ctx.Err() without waiting longer. Call it before closing the backend,
// so no reload writes to a closed one. The Cache keeps working afterwards: StaleWhileRevalidate
// answers stale results without reloading them.
func (c *Cache) Shutdown(ctx context.Context) error {
	c.bg.mu.Lock()
	c.bg.stopped = true
	c.bg.mu.Unlock()
	done := make(chan struct{})
	go func() {
		c.bg.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// background counts the reloads a Cache runs in the background.
type background struct {
	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
}

// start registers a background reload; it returns false once Shutdown was called. A true start is
// matched by one wg.Done.
func (b *background) start() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return false
	}
	b.wg.Add(1)
	return true
}
