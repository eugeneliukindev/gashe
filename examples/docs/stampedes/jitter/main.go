// Command stampedes/jitter: every TTL is spread randomly by up to 20%.
package main

import (
	"context"
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

	products := gashe.For(c, gashe.Policy[Product]{
		TTL: gashe.TTL{For: time.Hour, Jitter: 0.2},
	}) // 48 to 72 minutes each

	products.GetOrLoad(ctx, "7", func(context.Context) (Product, error) {
		return Product{Name: "Lamp"}, nil
	}) // Lamp, miss
}
