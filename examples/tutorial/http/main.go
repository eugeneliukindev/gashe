// Command http is step 7 of the tutorial: httpcache stores whole responses of a handler, so a
// repeated request does not run it.
package main

import (
	"context"
	"fmt"
	"log"
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
	listUsers := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		renders++ // a heavy page: queries and templates
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"42","name":"Ann"}]`))
	})

	cached := httpcache.New(c, httpcache.Options{Policy: gashe.Policy[httpcache.Response]{
		TTL:    time.Minute,
		Prefix: "page:",
	}})
	mux := http.NewServeMux()
	mux.Handle("GET /users", cached(listUsers))
	srv := httptest.NewServer(mux) // a real service: http.ListenAndServe(":8080", mux)
	defer srv.Close()

	get(srv.URL + "/users") // 200, Cache-Status: gashe; fwd=uri-miss; stored
	get(srv.URL + "/users") // 200, Cache-Status: gashe; hit
	fmt.Println("handler ran:", renders)
}

// get sends a GET request and prints how it was answered.
func get(url string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	fmt.Printf("GET /users: %d, Cache-Status: %s\n", resp.StatusCode, resp.Header.Get("Cache-Status"))
}
