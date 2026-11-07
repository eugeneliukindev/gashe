// Command httpserver caches whole responses of an HTTP handler with httpcache and prints how
// each request was answered: Cache-Status, ETag and 304.
package main

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	// --8<-- [start:api]
	renders := 0
	listProducts := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		renders++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"Mug"},{"name":"Kettle"}]`))
	})

	cached := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:    gashe.TTL{For: time.Minute},
			Prefix: "http:",
			Stale:  gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}}, // 5xx: last page
		},
	})
	mux := http.NewServeMux()
	mux.Handle("/products", cached(listProducts))
	srv := httptest.NewServer(mux) // or http.ListenAndServe(":8080", mux)
	defer srv.Close()
	url := srv.URL + "/products"

	resp, _ := http.Get(url) // 200, Cache-Status: gashe; fwd=uri-miss; stored
	etag := resp.Header.Get("ETag")

	http.Get(url) // 200, Cache-Status: gashe; hit

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	http.DefaultClient.Do(req) // 304, Cache-Status: gashe; hit, no body

	http.Post(url, "", nil) // 200, Cache-Status: gashe; fwd=method: never cached
	// renders is 2: the first GET and the POST
	// --8<-- [end:api]
}
