// Command login limits the calls of each client, and answers 429 over the limit.
package main

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		ctx, ip, key := r.Context(), r.RemoteAddr, r.Header.Get("X-Api-Key")

		err := c.RateLimit(ctx, "login:"+ip, gashe.FixedWindow{
			Calls:  5,
			Per:    time.Minute,
			BanFor: 15 * time.Minute,
		})
		if err == nil {
			// gashe.PerMinute(600) is gashe.SlidingWindow{Calls: 600, Per: time.Minute}
			err = c.RateLimit(ctx, "api-key:"+key, gashe.PerMinute(600))
		}

		var limited *gashe.RateLimitError // errors.Is(err, gashe.ErrRateLimited) holds too
		if errors.As(err, &limited) {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(limited.RetryAfter.Seconds()))))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
		}
	})
	http.ListenAndServe("localhost:8080", nil)
}
