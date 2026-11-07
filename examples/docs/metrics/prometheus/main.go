package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{Hooks: hooks})

	users := gashe.For(c, gashe.Policy[string]{TTL: gashe.TTL{For: time.Minute}})
	for _, id := range []string{"1", "1", "1", "2", "2", "3"} {
		_, _ = users.GetOrLoad(context.Background(), id, func(context.Context) (string, error) {
			time.Sleep(20 * time.Millisecond) // a query
			return "user " + id, nil
		})
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe("localhost:2112", nil))
}
