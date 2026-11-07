// Command httpserver caches whole responses of a handler with httpcache.
package main

import (
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func listProducts(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte(`[{"name":"Mug"}]`))
}

func main() {
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})
	cached := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{TTL: gashe.TTL{For: time.Minute}},
	})
	srv := httptest.NewServer(cached(http.HandlerFunc(listProducts))) // or http.ListenAndServe
	defer srv.Close()

	resp, _ := http.Get(srv.URL) // 200, Cache-Status: gashe; fwd=uri-miss; stored
	http.Get(srv.URL)            // 200, Cache-Status: gashe; hit: listProducts does not run

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("If-None-Match", resp.Header.Get("ETag"))
	http.DefaultClient.Do(req) // 304, Cache-Status: gashe; hit, no body

	http.Post(srv.URL, "", nil) // 200, Cache-Status: gashe; fwd=method: never cached
}
