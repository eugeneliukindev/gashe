// Command batch loads a list with GetOrLoadMany: one cache read and one loader call for all the
// keys that were missing, instead of one query per item.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type Product struct{ Name string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})

	db := map[string]Product{"1": {"Mug"}, "2": {"Kettle"}, "3": {"Spoon"}}
	loadProducts := func(_ context.Context, missing []string) (map[string]Product, error) {
		fmt.Println("  database query for", missing) // SELECT ... WHERE id = ANY($1)
		out := map[string]Product{}
		for _, id := range missing {
			if p, ok := db[id]; ok {
				out[id] = p
			}
		}
		return out, nil
	}

	for _, ids := range [][]string{{"1", "2"}, {"1", "2", "3"}, {"1", "2", "3", "99"}} {
		fmt.Println("page asks for", ids)
		products, err := gashe.GetOrLoadMany(ctx, c, ids, gashe.Policy[Product]{
			TTL:    time.Hour,
			Prefix: "product:",
		}, loadProducts)
		check(err)
		// got map[1:{Mug} 2:{Kettle}], after a query for [1 2]
		// got map[1:{Mug} 2:{Kettle} 3:{Spoon}], after a query for [3] only
		// got map[1:{Mug} 2:{Kettle} 3:{Spoon}], after a query for [99]: it does not exist, so it is left out
		fmt.Println("  got", products)
	}
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
