// Command batch loads a list with GetOrLoadMany: one cache read and one loader call for all the
// keys that were missing, instead of one query per item.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	db := map[string]string{"1": "Mug", "2": "Kettle", "3": "Spoon"}
	policy := gashe.Policy[string]{
		TTL:    time.Hour,
		Prefix: "product:",
	}
	// The loader gets only the ids the cache does not have.
	loadProducts := func(_ context.Context, missing []string) (map[string]string, error) {
		fmt.Println("  database query for", missing) // SELECT ... WHERE id = ANY($1)
		found := map[string]string{}
		for _, id := range missing {
			if name, ok := db[id]; ok {
				found[id] = name
			}
		}
		return found, nil
	}

	for _, ids := range [][]string{{"1", "2"}, {"1", "2", "3"}, {"1", "2", "3", "99"}} {
		fmt.Println("page asks for", ids)
		products, _ := gashe.GetOrLoadMany(ctx, c, ids, policy, loadProducts)
		// a query for [1 2], then for [3] only, then for [99]: it does not exist, so it is left out
		fmt.Println("  got", products)
	}
}
