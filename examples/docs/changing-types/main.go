package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type (
	UserV1 struct{ Name string }     // the type before the deploy
	UserV2 struct{ FullName string } // the type after it
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	// --8<-- [start:example]
	loadV1 := func(context.Context) (UserV1, error) { return UserV1{Name: "Ann"}, nil }
	loadV2 := func(context.Context) (UserV2, error) { return UserV2{FullName: "Ann Lee"}, nil }

	// Old processes.
	v1 := gashe.For(c, gashe.Policy[UserV1]{TTL: gashe.TTL{For: time.Hour}, Prefix: "users:"})
	v1.GetOrLoad(ctx, "42", loadV1) // miss
	v1.GetOrLoad(ctx, "42", loadV1) // hit

	// New processes, same prefix.
	v2 := gashe.For(c, gashe.Policy[UserV2]{TTL: gashe.TTL{For: time.Hour}, Prefix: "users:"})
	v2.GetOrLoad(ctx, "42", loadV2) // (1)!
	v1.GetOrLoad(ctx, "42", loadV1) // (2)!

	// New processes with users:v2: the versions keep apart.
	v2p := gashe.For(c, gashe.Policy[UserV2]{TTL: gashe.TTL{For: time.Hour}, Prefix: "users:v2:"})
	v2p.GetOrLoad(ctx, "42", loadV2) // miss
	v2p.GetOrLoad(ctx, "42", loadV2) // hit
	v1.GetOrLoad(ctx, "42", loadV1)  // hit: users:42 is left to the old processes
	// --8<-- [end:example]
}
