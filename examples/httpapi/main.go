// Command httpapi is an HTTP server whose pages are cached with httpcache; send it requests with
// curl to see every header. ADDR sets the address, localhost:8080 by default.
//
//	/products  a page cached per Accept-Language and the page parameter
//	/report    a page whose handler fails after the first render: Stale.IfError answers
//	/export    a slow page with at most one render at a time: a second request gets 503
//	/login     two calls a minute per client, then 429 with Retry-After for 15 minutes
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	products := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:    gashe.TTL{For: time.Minute},
			Prefix: "products:",
		},
		Vary: httpcache.Vary{
			Headers: []string{"Accept-Language"}, // the page is translated
			Query:   []string{"page"},            // utm_source and others make no new entry
		},
	})
	report := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:    gashe.TTL{For: time.Second},
			Prefix: "report:",
			Stale:  gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}}, // a 5xx gets the last good page
		},
	})
	export := httpcache.New(c, httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{
			TTL:      gashe.TTL{For: time.Minute},
			Stampede: gashe.Stampede{MaxLoads: 1}, // one render at a time in this process
			Prefix:   "export:",
		},
	})

	var reports atomic.Int64 // handlers run concurrently
	mux := http.NewServeMux()
	mux.Handle("/products", products(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		greeting := "Products"
		if r.Header.Get("Accept-Language") == "ru" {
			greeting = "Товары"
		}
		json.NewEncoder(w).Encode(map[string]any{
			"title": greeting,
			"page":  cmp.Or(r.URL.Query().Get("page"), "1"),
			"items": []string{"Mug", "Kettle"},
		})
	})))
	mux.Handle("/report", report(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if reports.Add(1) > 1 {
			http.Error(w, "database is down", http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, "report: 42 orders")
	})))
	exportPage := export(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second) // a heavy page
		fmt.Fprintln(w, "export done")
	}))
	mux.HandleFunc("/export", func(w http.ResponseWriter, r *http.Request) {
		// 2.5 seconds for the whole request, the render included: a request that waits for the
		// render slot past it gets 503 instead of piling up.
		ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
		defer cancel()
		exportPage.ServeHTTP(w, r.WithContext(ctx))
	})

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		err := c.RateLimit(r.Context(), "login:"+host, gashe.FixedWindow{
			Calls:  2,
			Per:    time.Minute,
			BanFor: 15 * time.Minute,
		})
		var limited *gashe.RateLimitError
		if errors.As(err, &limited) {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
			http.Error(w, "too many attempts", http.StatusTooManyRequests)
			return
		}
		fmt.Fprintln(w, "login form")
	})

	addr := cmp.Or(os.Getenv("ADDR"), "localhost:8080")
	log.Printf("listening on %q", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
