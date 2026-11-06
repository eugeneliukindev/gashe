# Export metrics { #export-metrics }

Export the hit ratio and the loader latency of the cache to Prometheus or OpenTelemetry.

## The code { #the-code }

gashe has no dependencies: the metrics library is in your code, fed by `Options.OnResult` and
`Options.OnError`.

```go
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

var (
	cacheResults = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gashe_results_total",
		Help: "GetOrLoad calls by how they were answered.",
	}, []string{"result"}) // hit, miss, stale-if-error, ...: never the key, it has no bound
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

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{
		OnResult: func(_ context.Context, o gashe.Outcome) {
			cacheResults.WithLabelValues(o.Result.String()).Inc()
			if o.Load > 0 { // the loader ran in this call
				cacheLoads.Observe(o.Load.Seconds())
			}
		},
		OnError: func(_ context.Context, err error) {
			cacheErrors.Inc()
			log.Println("cache:", err) // the metric says how often, the log says what
		},
	})

	users := gashe.For(c, gashe.Policy[string]{TTL: time.Minute})
	ctx := context.Background()
	for _, id := range []string{"1", "1", "1", "2", "2", "3"} {
		_, _ = users.GetOrLoad(ctx, id, func(context.Context) (string, error) {
			time.Sleep(20 * time.Millisecond) // a query
			return "user " + id, nil
		})
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe("localhost:2112", nil))
}
```

`curl localhost:2112/metrics | grep ^gashe` after it ran (the buckets left out):

```text
gashe_errors_total 0
gashe_load_seconds_sum 0.060567888
gashe_load_seconds_count 3
gashe_results_total{result="hit"} 3
gashe_results_total{result="miss"} 3
```

## How it works { #how-it-works }

| Metric | From | Use it for |
|---|---|---|
| `gashe_results_total{result}` | `Outcome.Result`, every `GetOrLoad` and every key of `GetOrLoadMany` | hit ratio; how often old data answers (`stale-if-error`, `fallback`) |
| `gashe_load_seconds` | `Outcome.Load`, when the loader ran | what a miss costs: pick TTLs and `LockOnMiss` by it |
| `gashe_errors_total` | `OnError`: backend failures, old results answered instead of an error | alerts: the cache works around a problem |

- **Hit ratio** in PromQL, anything the loader did not answer counted as a hit:
  `sum(rate(gashe_results_total{result!="miss"}[5m])) / sum(rate(gashe_results_total[5m]))`.
- **Labels:** never the key: it has no bound, and every key would make a series.
- **Cost:** both run synchronously on every call: a counter is fine, a network call is not.
- **Pods:** each process counts its own calls; sum them in the query.
  More on the hooks: [`OnResult`, `OnError`](../concepts/setup.md#onresult-onerror-monitoring).

## OpenTelemetry { #opentelemetry }

The same three instruments on the global `MeterProvider`; your OpenTelemetry setup exports them.

```go
package main

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// cacheOptions reports to the global MeterProvider, which your OpenTelemetry setup configures.
func cacheOptions() (gashe.Options, error) {
	meter := otel.Meter("gashe")
	results, err := meter.Int64Counter("gashe.results", metric.WithDescription("GetOrLoad calls by how they were answered"))
	if err != nil {
		return gashe.Options{}, err
	}
	loads, err := meter.Float64Histogram("gashe.load.duration", metric.WithUnit("s"))
	if err != nil {
		return gashe.Options{}, err
	}
	errs, err := meter.Int64Counter("gashe.errors")
	if err != nil {
		return gashe.Options{}, err
	}
	return gashe.Options{
		OnResult: func(ctx context.Context, o gashe.Outcome) {
			results.Add(ctx, 1, metric.WithAttributes(attribute.String("result", o.Result.String())))
			if o.Load > 0 {
				loads.Record(ctx, o.Load.Seconds())
			}
		},
		OnError: func(ctx context.Context, _ error) { errs.Add(ctx, 1) },
	}, nil
}

func main() {
	o, err := cacheOptions()
	if err != nil {
		panic(err)
	}
	mem := memory.New(memory.Options{})
	defer mem.Close()
	_ = gashe.New(mem, o)
}
```

## Variations { #variations }

| If you need | Do |
|---|---|
| numbers per kind of data | a label from the prefix: `kind, _, _ := strings.Cut(o.Key, ":")`, if every `Policy.Prefix` is one fixed word |
| traces of slow loads | start a span in the loader: it gets the `ctx` of the call |
| to see why the cache degraded | log the `err` of `OnError` as well; the metric says how often, the log says what |
