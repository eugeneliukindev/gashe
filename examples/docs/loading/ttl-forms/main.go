// Command ttl-forms shows the forms of TTL.For: a duration, a string, a function of the result.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Token struct{ ExpiresAt time.Time }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	cfg := struct{ UserTTL string }{UserTTL: "10m"}
	loadToken := func(context.Context) (Token, error) {
		return Token{ExpiresAt: time.Now().Add(time.Hour)}, nil
	}

	policies := []gashe.Policy[Token]{
		{TTL: gashe.TTL{For: time.Hour}},
		{TTL: gashe.TTL{For: "1h30m"}},
		{TTL: gashe.TTL{For: cfg.UserTTL}}, // (1)!
		{TTL: gashe.TTL{For: func(t Token) time.Duration { return time.Until(t.ExpiresAt) - time.Minute }}},
		{TTL: gashe.TTL{For: time.Hour, Jitter: 0.1}}, // (2)!
		{TTL: gashe.TTL{For: 3600}},                   // (3)!
	}
	for _, p := range policies {
		gashe.GetOrLoad(ctx, c, "token", p, loadToken)
	}
}
