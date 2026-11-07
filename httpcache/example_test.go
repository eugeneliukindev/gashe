package httpcache_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func ExampleNew() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Println("rendering")
		_, _ = fmt.Fprint(w, "<h1>Prices</h1>")
	})
	h := httpcache.New(
		c,
		httpcache.Options{Policy: gashe.Policy[httpcache.Response]{TTL: gashe.TTL{For: time.Minute}}},
	)(
		page,
	)

	for range 2 {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, "/prices", http.NoBody))
		fmt.Println(w.Code, w.Header().Get("Cache-Status"))
	}
	// Output:
	// rendering
	// 200 gashe; fwd=uri-miss; stored
	// 200 gashe; hit
}

func ExampleVary() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("page"))
		_, _ = fmt.Fprint(w, "page ", n)
	})
	h := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{TTL: gashe.TTL{For: time.Minute}},
		Vary: httpcache.Vary{
			Query:   []string{"page"},            // utm_source and the like do not make new entries
			Headers: []string{"Accept-Language"}, // a page per language
		},
	})(page)

	for _, url := range []string{"/news?page=2", "/news?page=2&utm_source=mail", "/news?page=3"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody))
		fmt.Println(w.Body, "|", w.Header().Get("Cache-Status"))
	}
	// Output:
	// page 2 | gashe; fwd=uri-miss; stored
	// page 2 | gashe; hit
	// page 3 | gashe; fwd=uri-miss; stored
}
