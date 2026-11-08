// Command delete deletes one entry, so the next GetOrLoad loads again.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	// --8<-- [start:example]
	userCache := gashe.For(c, gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "users:v2:"})

	existed, err := userCache.Delete(ctx, "42") // deletes users:v2:42
	_, _ = existed, err
	// --8<-- [end:example]
}
