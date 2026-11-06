// Command httpserver caches whole responses of an HTTP handler with httpcache and prints how
// each request was answered: Cache-Status, ETag and 304.
package main

import (
	"fmt"
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

	renders := 0
	listProducts := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		renders++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"Mug"},{"name":"Kettle"}]`))
	})

	cached := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:          time.Minute,
			StaleIfError: time.Hour, // a 5xx gets the last good page
			Prefix:       "http:",
		},
	})
	mux := http.NewServeMux()
	mux.Handle("/products", cached(listProducts))
	srv := httptest.NewServer(mux) // a real server would be http.ListenAndServe(":8080", mux)
	defer srv.Close()
	url := srv.URL + "/products"

	resp, _ := http.Get(url)
	etag := resp.Header.Get("ETag")
	fmt.Println("GET:", resp.StatusCode, resp.Header.Get("Cache-Status"), "ETag:", etag) // fwd=uri-miss; stored

	resp, _ = http.Get(url)
	fmt.Println("GET:", resp.StatusCode, resp.Header.Get("Cache-Status")) // hit

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	resp, _ = http.DefaultClient.Do(req)
	fmt.Println("GET If-None-Match:", resp.StatusCode, resp.Header.Get("Cache-Status")) // 304 with no body

	resp, _ = http.Post(url, "", nil)
	fmt.Println("POST:", resp.StatusCode, resp.Header.Get("Cache-Status")) // fwd=method: never cached

	fmt.Println("handler ran", renders, "times") // handler ran 2 times: the first GET and the POST
}
