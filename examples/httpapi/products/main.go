// Command products serves a page cached per Accept-Language and ?page; send it requests with curl.
package main

import (
	"cmp"
	"encoding/json"
	"net/http"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func listProducts(w http.ResponseWriter, r *http.Request) {
	title := map[string]string{"ru": "Товары"}[r.Header.Get("Accept-Language")]
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"title": cmp.Or(title, "Products"),
		"page":  cmp.Or(r.URL.Query().Get("page"), "1"),
		"items": []string{"Mug", "Kettle"},
	})
}

func main() {
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})
	cached := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:    gashe.TTL{For: time.Minute},
			Prefix: "products:",
		},
		Vary: httpcache.Vary{
			Query:   []string{"page"},            // utm_source and others make no new entry
			Headers: []string{"Accept-Language"}, // the page is translated
		},
	})
	http.Handle("/products", cached(http.HandlerFunc(listProducts)))
	http.ListenAndServe("localhost:8080", nil)
}
