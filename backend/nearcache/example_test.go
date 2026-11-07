package nearcache_test

import (
	"context"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backend/nearcache"
)

func ExampleNew() {
	ctx := context.Background()
	local := memory.New(memory.Options{MaxEntries: 100})
	defer local.Close()
	shared := memory.New(memory.Options{}) // usually Redis
	defer shared.Close()
	near := nearcache.New(local, shared, nearcache.Options{LocalTTL: time.Minute})
	defer func() { _ = near.Close() }()
	c := gashe.New(near, gashe.Options{})

	_ = c.Set(ctx, "user:42", "Ann", time.Hour) // goes to shared
	var name string
	_ = c.Get(ctx, "user:42", &name) // copies the value to local
	fmt.Println(name, local.Len(), shared.Len())

	_, _ = shared.Delete(ctx, "user:42") // as another process without a Bus would
	_ = c.Get(ctx, "user:42", &name)     // still answered from local for up to LocalTTL
	fmt.Println(name)
	// Output:
	// Ann 1 1
	// Ann
}
