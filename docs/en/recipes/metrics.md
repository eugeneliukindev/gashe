# Export metrics { #export-metrics }

Export the hit ratio and the loader latency of the cache to Prometheus or OpenTelemetry.

## The code { #the-code }

gashe has no dependencies: the metrics library is in your code, fed by `Options.Hooks.OnResult` and
`Options.Hooks.OnError`. The metrics and the hooks, `metrics.go`:

```go
--8<-- "examples/docs/metrics/prometheus/metrics.go"
```

1. The label is hit, miss, stale-if-error, ...: never the key, it has no bound.
2. The metric says how often, the log says what.

A cache with the hooks, six calls and the `/metrics` page, `main.go`:

```go
--8<-- "examples/docs/metrics/prometheus/main.go"
```

The metrics after it ran, the buckets left out:

```bash title="Request"
curl localhost:2112/metrics | grep ^gashe
```

```text title="Response"
gashe_errors_total 0
gashe_load_seconds_sum 0.060567888
gashe_load_seconds_count 3
gashe_results_total{result="hit"} 3
gashe_results_total{result="miss"} 3
```

## How it works { #how-it-works }

| Metric | From | Use it for |
|---|---|---|
| `gashe_results_total{result}` | `Outcome.Result`, every `GetOrLoad` and every key of `GetOrLoadMany` | hit ratio; how often a stale result or the fallback answers (`stale-if-error`, `fallback`) |
| `gashe_load_seconds` | `Outcome.Load`, when the loader ran | what a miss costs: pick TTLs and `Stampede.LockFor` by it |
| `gashe_errors_total` | `OnError`: backend failures, stale results answered instead of an error | alerts: the cache works around a problem |

| Point | What to know |
|---|---|
| Hit ratio | in PromQL, anything the loader did not answer counted as a hit: `sum(rate(gashe_results_total{result!="miss"}[5m])) / sum(rate(gashe_results_total[5m]))` |
| Labels | never the key: it has no bound, and every key would make a series |
| Cost | both hooks run synchronously on every call: a counter is fine, a network call is not |
| Processes | each process has its own counters in its own memory; sum them in the query |

More on the hooks: [`OnResult`, `OnError`](../concepts/setup.md#onresult-onerror).

## OpenTelemetry { #opentelemetry }

The same three instruments on the global `MeterProvider`; your OpenTelemetry setup exports them.

```go
--8<-- "examples/docs/metrics/opentelemetry/main.go"
```

## Variations { #variations }

| If you need | Do |
|---|---|
| numbers per kind of data | a label from the prefix: `kind, _, _ := strings.Cut(o.Key, ":")`, if every `Policy.Prefix` is one fixed word |
| traces of slow loads | start a span in the loader: it gets the `ctx` of the call |
| to see why the cache degraded | log the `err` of `OnError` as well; the metric says how often, the log says what |
