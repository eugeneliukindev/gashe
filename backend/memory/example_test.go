package memory_test

import (
	"context"
	"fmt"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func ExampleNew() {
	ctx := context.Background()
	mem := memory.New(memory.Options{MaxEntries: 2})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	for _, key := range []string{"a", "b", "c"} {
		_ = c.Set(ctx, key, 1, 0)
	}
	a, _ := c.Exists(ctx, "a") // the least recently used entry was evicted
	fmt.Println(mem.Len(), a)
	// Output:
	// 2 false
}
