package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	var down atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { // a stand-in for the real API
		switch {
		case down.Load():
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.URL.Query().Get("city") != "Paris":
			w.WriteHeader(http.StatusNotFound)
		default:
			_, _ = fmt.Fprint(w, `{"city":"Paris","temp":18.5}`)
		}
	}))
	defer api.Close()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{Hooks: gashe.Hooks{OnError: func(_ context.Context, err error) { log.Println(err) }}})
	weather := NewWeatherAPI(c, api.URL, 100*time.Millisecond) // a short TTL to see it expire
	weather.Get(ctx, "Paris")                                  // {Paris 18.5}: a miss, the API is called
	weather.Get(ctx, "Atlantis")                               // ErrNoCity: a 404, cached
	weather.Get(ctx, "Atlantis")                               // ErrNoCity again, no call
	down.Store(true)
	time.Sleep(150 * time.Millisecond) // TTL passes while the API is down
	weather.Get(ctx, "Paris")          // {Paris 18.5}: the API fails, the last answer; OnError logs it
}
