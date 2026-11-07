// Command hooks feeds Prometheus metrics from Options.Hooks.OnResult.
package main

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

var (
	results = promauto.NewCounterVec(prometheus.CounterOpts{Name: "cache_results_total"}, []string{"result"})
	loads   = promauto.NewHistogram(prometheus.HistogramOpts{Name: "cache_load_seconds"})
)

func main() {
	backend := memory.New(memory.Options{})
	defer backend.Close()

	c := gashe.New(backend, gashe.Options{
		Hooks: gashe.Hooks{
			OnResult: func(ctx context.Context, o gashe.Outcome) {
				results.WithLabelValues(o.Result.String()).Inc() // hit ratio
				loads.Observe(o.Load.Seconds())                  // (1)!
			},
		},
	})

	load := func(context.Context) (string, error) { return "Ann", nil }
	gashe.GetOrLoad(context.Background(), c, "user:42", gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}}, load)
}
