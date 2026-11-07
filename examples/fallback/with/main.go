// Command fallback: with Policy.Fallback a lost Redis costs one load per key.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// down stands for a lost Redis: every read fails, as with a lost connection.
type down struct{ gashe.Backend }

func (down) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("redis: connection refused")
}

func main() {
	ctx := context.Background()
	c := gashe.New(down{memory.New(memory.Options{})}, gashe.Options{})
	policy := gashe.Policy[string]{
		TTL:      gashe.TTL{For: time.Hour},
		Fallback: gashe.Fallback{Backend: memory.New(memory.Options{}), MaxTTL: 30 * time.Second},
	}

	queries := 0
	for range 100 { // 100 requests while Redis is down
		gashe.GetOrLoad(ctx, c, "user:42", policy, func(context.Context) (string, error) { queries++; return "Ann", nil })
	}
	// queries is 1: the result comes from Fallback
}
