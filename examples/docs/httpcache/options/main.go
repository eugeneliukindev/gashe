// Command options caches the responses of a handler, a key per language, page, sort and currency.
package main

import (
	"net/http"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func listProducts(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte(`[{"name":"Mug"}]`))
}

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	mux := http.NewServeMux()

	cached := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:   gashe.TTL{For: time.Minute},
			Stale: gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}},
		},
		Vary: httpcache.Vary{
			Headers: []string{"Accept-Language"},
			Query:   []string{"page", "sort"},
			Cookies: []string{"currency"},
		},
	})
	mux.Handle("GET /products/", cached(http.HandlerFunc(listProducts)))
}
