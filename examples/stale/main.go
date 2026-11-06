// Command stale shows the three ways a stale result answers: while it is reloaded in the
// background (StaleWhileRevalidate), when the source fails (StaleIfError), and when the source
// is slow (StaleIfSlow).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// source stands for a database whose answers, failures and delays the example controls.
type source struct {
	version int
	fail    bool
	delay   time.Duration
}

func (s *source) load(context.Context) (int, error) {
	time.Sleep(s.delay)
	if s.fail {
		return 0, errors.New("database is down")
	}
	s.version++
	return s.version, nil
}

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{OnResult: func(_ context.Context, o gashe.Outcome) {
		fmt.Printf("  %s: %s\n", o.Key, o.Result)
	}})
	defer func() { _ = c.Shutdown(ctx) }()

	fmt.Println("stale-while-revalidate:")
	db := &source{}
	swr := gashe.Policy[int]{TTL: time.Second, StaleWhileRevalidate: time.Minute}
	get(ctx, c, "swr", swr, db) // swr: miss, got v1
	time.Sleep(1100 * time.Millisecond)
	get(ctx, c, "swr", swr, db) // swr: stale-while-revalidate, got v1 at once; v2 loads in the background
	time.Sleep(100 * time.Millisecond)
	get(ctx, c, "swr", swr, db) // swr: hit, got v2

	fmt.Println("stale-if-error:")
	db = &source{}
	sie := gashe.Policy[int]{TTL: time.Second, StaleIfError: time.Minute}
	get(ctx, c, "sie", sie, db) // sie: miss, got v1
	time.Sleep(1100 * time.Millisecond)
	db.fail = true
	get(ctx, c, "sie", sie, db) // sie: stale-if-error, got v1 instead of the error

	fmt.Println("stale-if-slow:")
	db = &source{}
	sis := gashe.Policy[int]{
		TTL:          time.Second,
		StaleIfError: time.Minute,
		StaleIfSlow:  100 * time.Millisecond,
	}
	get(ctx, c, "sis", sis, db) // sis: miss, got v1
	time.Sleep(1100 * time.Millisecond)
	db.delay = time.Second
	get(ctx, c, "sis", sis, db) // sis: stale-if-slow, got v1 in 100ms; v2 goes on loading
	time.Sleep(1100 * time.Millisecond)
	get(ctx, c, "sis", sis, db) // sis: hit, got v2
}

func get(ctx context.Context, c *gashe.Cache, key string, o gashe.Policy[int], db *source) {
	start := time.Now()
	v, err := gashe.GetOrLoad(ctx, c, key, o, db.load)
	check(err)
	fmt.Printf("  got v%d in %v\n", v, time.Since(start).Round(10*time.Millisecond))
}

// check stops the example on an error it does not expect.
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
