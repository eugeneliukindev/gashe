package main

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func cacheOptions() (gashe.Options, error) {
	meter := otel.Meter("gashe")
	results, err1 := meter.Int64Counter("gashe.results",
		metric.WithDescription("GetOrLoad calls by how they were answered"))
	loads, err2 := meter.Float64Histogram("gashe.load.duration", metric.WithUnit("s"))
	errs, err3 := meter.Int64Counter("gashe.errors")
	if err := errors.Join(err1, err2, err3); err != nil {
		return gashe.Options{}, err
	}
	return gashe.Options{
		Hooks: gashe.Hooks{OnResult: func(ctx context.Context, o gashe.Outcome) {
			results.Add(ctx, 1, metric.WithAttributes(attribute.String("result", o.Result.String())))
			if o.Load > 0 {
				loads.Record(ctx, o.Load.Seconds())
			}
		}, OnError: func(ctx context.Context, _ error) { errs.Add(ctx, 1) }},
	}, nil
}

func main() {
	// --8<-- [start:example]
	o, err := cacheOptions()
	if err != nil {
		panic(err)
	}
	_ = gashe.New(memory.New(memory.Options{}), o)
	// --8<-- [end:example]
}
