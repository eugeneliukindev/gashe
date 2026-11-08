// Command report serves a page whose handler fails after its first render.
package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

var renders atomic.Int64 // handlers run concurrently

func report(w http.ResponseWriter, _ *http.Request) {
	if renders.Add(1) > 1 {
		http.Error(w, "database is down", http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, "report: 42 orders")
}

func main() {
	// --8<-- [start:example]
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})
	cached := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:    gashe.TTL{For: time.Second},
			Prefix: "report:",
			Stale:  gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}}, // a 5xx gets the last page
		},
	})
	http.Handle("/report", cached(http.HandlerFunc(report)))
	http.ListenAndServe("localhost:8080", nil)
	// --8<-- [end:example]
}
