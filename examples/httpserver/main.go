// Command httpserver caches whole responses of an HTTP handler with httpcache and prints how
// each request was answered: Cache-Status, ETag and 304.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
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
		time.Sleep(100 * time.Millisecond) // a heavy page
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"Mug"},{"name":"Kettle"}]`))
	})

	cached := httpcache.New(c, httpcache.Options{Policy: gashe.Policy[httpcache.Response]{
		Prefix:       "http:",
		TTL:          time.Minute,
		StaleIfError: time.Hour, // a 5xx gets the last good page
	}})
	mux := http.NewServeMux()
	mux.Handle("/products", cached(listProducts))
	srv := httptest.NewServer(mux) // a real server would be http.ListenAndServe(":8080", mux)
	defer srv.Close()

	etag := request(srv.URL, http.MethodGet, "") // GET 200 Cache-Status: "gashe; fwd=uri-miss; stored"
	request(srv.URL, http.MethodGet, "")         // GET 200 Cache-Status: "gashe; hit"
	request(srv.URL, http.MethodGet, etag)       // GET 304 Cache-Status: "gashe; hit", no body
	request(srv.URL, http.MethodPost, "")        // POST 200 Cache-Status: "gashe; fwd=method"
	fmt.Println("handler ran", renders, "times") // handler ran 2 times: the first GET and the POST
}

// request sends one request, prints how it was answered and returns its ETag.
func request(url, method, ifNoneMatch string) string {
	req, err := http.NewRequestWithContext(context.Background(), method, url+"/products", strings.NewReader(""))
	if err != nil {
		log.Fatal(err)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	fmt.Printf("%-4s %d  Cache-Status: %-30q ETag: %s\n", method, resp.StatusCode,
		resp.Header.Get("Cache-Status"), resp.Header.Get("ETag"))
	return resp.Header.Get("ETag")
}
