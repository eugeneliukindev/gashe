// Command redis is step 5 of the tutorial: three pods share one Redis. A cold key loads once per
// pod; with LockOnMiss it loads once for all of them. A memory backend shared by the pods stands
// in for Redis, so the program runs without a server.
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct {
	ID   string
	Name string
}

func main() {
	run("without LockOnMiss:", gashe.Policy[User]{TTL: time.Hour, Prefix: "user:"})
	run("with LockOnMiss:", gashe.Policy[User]{TTL: time.Hour, LockOnMiss: 5 * time.Second, Prefix: "user:"})
}

// run starts three pods on one shared backend; each asks for the same cold user at once.
func run(name string, policy gashe.Policy[User]) {
	ctx := context.Background()
	// On Redis: backend := gasheredis.New(goredis.NewClient(&goredis.Options{Addr: "localhost:6379"}))
	backend := memory.New(memory.Options{})
	defer backend.Close()

	var queries atomic.Int32
	loadUser := func(context.Context) (User, error) {
		queries.Add(1)
		time.Sleep(100 * time.Millisecond) // a slow query
		return User{ID: "42", Name: "Ann"}, nil
	}

	var wg sync.WaitGroup
	for range 3 {
		c := gashe.New(backend, gashe.Options{}) // one Cache per pod
		users := gashe.For(c, policy)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = users.GetOrLoad(ctx, "42", loadUser)
		}()
	}
	wg.Wait()
	fmt.Printf("%-20s 3 pods, database queries: %d\n", name, queries.Load())
}
