// Command getorloadmany loads a list of products with one cache read and one query.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Product struct{ Name string }

// productsByID stands for the database.
func productsByID(_ context.Context, ids []string) (map[string]Product, error) {
	db := map[string]Product{"1": {"Mug"}, "2": {"Kettle"}, "3": {"Spoon"}, "4": {"Fork"}}
	found := map[string]Product{}
	for _, id := range ids {
		if p, ok := db[id]; ok {
			found[id] = p
		}
	}
	return found, nil
}

func main() {
	ctx := context.Background()
	// --8<-- [start:example]
	c := gashe.New(memory.New(memory.Options{}), gashe.Options{})

	productPolicy := gashe.Policy[Product]{TTL: gashe.TTL{For: time.Hour}, Prefix: "product:"}
	productCache := gashe.For(c, productPolicy)
	loadProducts := productsByID
	ids := []string{"1", "2", "3", "4"}

	productCache.GetOrLoadMany(ctx, ids, func(ctx context.Context, missing []string) (map[string]Product, error) {
		return productsByID(ctx, missing) // SELECT ... WHERE id = ANY($1)
	})
	gashe.GetOrLoadMany(ctx, c, ids, productPolicy, loadProducts) // the same without For
	// --8<-- [end:example]
}
