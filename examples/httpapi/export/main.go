// Command export serves a slow page with at most one render at a time.
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func export(w http.ResponseWriter, _ *http.Request) {
	time.Sleep(2 * time.Second) // a heavy page
	fmt.Fprintln(w, "export done")
}

func main() {
	// --8<-- [start:example]
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})
	cached := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:      gashe.TTL{For: time.Minute},
			Prefix:   "export:",
			Stampede: gashe.Stampede{SingleFlight: true, MaxLoads: 1}, // one render at a time
		},
	})
	page := cached(http.HandlerFunc(export))
	http.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		// 2.5 seconds per request, the render included: a request still waiting then gets 503
		ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
		defer cancel()
		page.ServeHTTP(w, r.WithContext(ctx))
	})
	http.ListenAndServe("localhost:8080", nil)
	// --8<-- [end:example]
}
