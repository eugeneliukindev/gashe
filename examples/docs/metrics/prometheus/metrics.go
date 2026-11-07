package main

import (
	"context"
	"log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/eugeneliukindev/gashe"
)

var (
	cacheResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gashe_results_total",
		Help: "GetOrLoad calls by how they were answered.",
	}, []string{"result"}) // (1)!
	cacheLoads = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "gashe_load_seconds",
		Help:    "How long the loader ran on a miss.",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 14), // 1ms to 8s
	})
	cacheErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gashe_errors_total",
		Help: "Errors the cache worked around: backend failures, old results answered.",
	})
)

// hooks feed the metrics on every call of the cache.
var hooks = gashe.Hooks{OnResult: func(_ context.Context, o gashe.Outcome) {
	cacheResults.WithLabelValues(o.Result.String()).Inc()
	if o.Load > 0 { // the loader ran in this call
		cacheLoads.Observe(o.Load.Seconds())
	}
}, OnError: func(_ context.Context, err error) {
	cacheErrors.Inc()
	log.Println("cache:", err) // (2)!
}}
