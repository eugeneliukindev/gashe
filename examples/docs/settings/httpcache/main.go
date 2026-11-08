// Command httpcache sets every field of httpcache.Options and httpcache.Vary.
package main

import (
	"context"
	"net/http"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func main() {
	backend := memory.New(memory.Options{})
	defer backend.Close()
	c := gashe.New(backend, gashe.Options{})
	// --8<-- [start:example]
	defer c.Shutdown(context.Background())

	cached := httpcache.New(c, httpcache.Options{
		// as for GetOrLoad; every field of Policy applies but Negative
		Policy: gashe.Policy[httpcache.Response]{
			TTL: gashe.TTL{For: time.Minute},
			// which statuses are stored; nil If stores 200 only
			Store: gashe.Store[httpcache.Response]{
				If: func(r httpcache.Response) bool {
					return r.Status == 200 || r.Status == 404
				},
			},
		},
		// what a page depends on, besides its host and path
		Vary: httpcache.Vary{
			// query parameters in the key; nil means the whole query
			Query: []string{"page"},
			// request headers in the key; nil means none
			Headers: []string{"Accept-Language"},
			// cookies in the key; nil sends requests with cookies past
			Cookies: []string{"currency"},
			// who the page is for; nil means pages are shared
			User: func(r *http.Request) string {
				return r.Header.Get("X-User-ID")
			},
			// anything else, at the end of the key; nil means none
			Custom: func(r *http.Request) string {
				return ""
			},
		},
		MaxBodyBytes: 512 << 10, // the largest body stored; 0 means 1 MB
	})

	_ = cached(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	// --8<-- [end:example]
}
