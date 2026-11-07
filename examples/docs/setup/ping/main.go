// Command ping checks that the backend is reachable, as a health check does.
package main

import (
	"context"
	"log"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func main() {
	ctx := context.Background()
	backend := memory.New(memory.Options{})
	defer backend.Close()
	c := gashe.New(backend, gashe.Options{})

	err := c.Ping(ctx)
	if err != nil {
		log.Fatal("cache is down: ", err)
	}
}
