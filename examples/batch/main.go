// Command batch loads a list with GetOrLoadMany: one cache read and one loader call for all the
// keys that were missing, instead of one query per item.
package main

import (
	"context"
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

	// --8<-- [start:api]
	policy := gashe.Policy[string]{
		TTL:    gashe.TTL{For: time.Hour},
		Prefix: "product:",
	}
	// The loader gets only the ids the cache does not have.
	loadProducts := func(_ context.Context, ids []string) (map[string]string, error) {
		found := map[string]string{} // SELECT ... WHERE id = ANY($1)
		for _, id := range ids {
			if name, ok := db[id]; ok {
				found[id] = name
			}
		}
		return found, nil
	}

	gashe.GetOrLoadMany(ctx, c, []string{"1", "2"}, policy, loadProducts)      // loads 1, 2
	gashe.GetOrLoadMany(ctx, c, []string{"1", "2", "3"}, policy, loadProducts) // loads 3
	gashe.GetOrLoadMany(ctx, c, []string{"1", "99"}, policy, loadProducts)     // loads 99: absent
	// --8<-- [end:api]
}
