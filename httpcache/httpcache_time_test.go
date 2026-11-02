//go:build go1.25

package httpcache_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/eugeneliukindev/gashe/httpcache"
)

// slowPage blocks every run until release is closed, so concurrent requests overlap.
type slowPage struct {
	runs    atomic.Int32
	release chan struct{}
	cookie  bool
}

func (p *slowPage) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	n := p.runs.Add(1)
	<-p.release
	if p.cookie {
		w.Header().Set("Set-Cookie", fmt.Sprintf("session=%d", n))
	}
	_, _ = w.Write([]byte("page"))
}

// concurrently sends n requests for one URL while the handler blocks and returns the responses.
func concurrently(t *testing.T, h http.Handler, p *slowPage, n int) []*httptest.ResponseRecorder {
	t.Helper()
	out := make([]*httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	for i := range out {
		out[i] = httptest.NewRecorder()
		wg.Go(func() {
			h.ServeHTTP(out[i], httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody))
		})
	}
	synctest.Wait() // one request renders, the others wait for it
	close(p.release)
	wg.Wait()
	return out
}

func TestConcurrentRequestsShareOneRender(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &slowPage{release: make(chan struct{})}
		h := serve(t, newCache(t), &httpcache.Options{}, p)

		for _, w := range concurrently(t, h, p, 5) {
			if w.Body.String() != "page" {
				t.Fatalf("body %q", w.Body.String())
			}
		}
		if p.runs.Load() != 1 {
			t.Fatalf("handler ran %d times for 5 concurrent requests, want 1", p.runs.Load())
		}
	})
}

func TestConcurrentRequestsNeverShareAResponseThatIsNotStored(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &slowPage{release: make(chan struct{}), cookie: true}
		h := serve(t, newCache(t), &httpcache.Options{}, p)

		seen := map[string]bool{}
		for _, w := range concurrently(t, h, p, 5) {
			cookie := w.Header().Get("Set-Cookie")
			if cookie == "" || seen[cookie] {
				t.Fatalf("cookie %q was given to two requests", cookie)
			}
			seen[cookie] = true
		}
	})
}
