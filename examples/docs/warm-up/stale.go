package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
)

var stalePolicy = gashe.Policy[Product]{
	TTL:      gashe.TTL{For: time.Hour},
	Stale:    gashe.Stale{IfError: gashe.StaleIfError{For: time.Hour}},
	Stampede: gashe.Stampede{LockFor: 5 * time.Second},
}

func warmStale(ctx context.Context, c *gashe.Cache, batch []string,
	query func(ctx context.Context, ids []string) (map[string]Product, error),
) error {
	warm := stalePolicy // (1)!
	warm.Stale = gashe.Stale{}
	warm.Stampede.LockFor = 0
	warm.Negative = gashe.Negative{}
	_, err := gashe.GetOrLoadMany(ctx, c, batch, warm, query)
	return err
}
