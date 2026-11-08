// Command login allows two calls a minute per client, then answers 429 for 15 minutes.
package main

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	// --8<-- [start:example]
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
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
	http.ListenAndServe("localhost:8080", nil)
	// --8<-- [end:example]
}
