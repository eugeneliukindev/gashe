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
		Policy: gashe.Policy[httpcache.Response]{ // as for GetOrLoad; every field of Policy applies but Negative
			TTL: gashe.TTL{For: time.Minute},
			Store: gashe.Store[httpcache.Response]{ // which statuses are stored; nil If stores 200 only
				If: func(r httpcache.Response) bool { return r.Status == 200 || r.Status == 404 },
			},
		},
		Vary: httpcache.Vary{ // what a page depends on, besides its host and path
			Query:   []string{"page"},            // query parameters in the key; nil means the whole query
			Headers: []string{"Accept-Language"}, // request headers in the key; nil means none
			Cookies: []string{"currency"},        // cookies in the key; nil sends requests with cookies past
			User: func(r *http.Request) string { // who the page is for; nil means pages are shared
				return r.Header.Get("X-User-ID")
			},
			Custom: func(r *http.Request) string { // anything else, at the end of the key; nil means none
				return ""
			},
		},
		MaxBodyBytes: 512 << 10, // the largest body stored; 0 means 1 MB
	})

	_ = cached(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) }))
	// --8<-- [end:example]
}
