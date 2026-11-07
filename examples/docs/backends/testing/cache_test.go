// Package users shows a cache for tests, and a backend that fails on purpose.
package users

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

func newCache(*testing.T) *gashe.Cache {
	backend := memory.New(memory.Options{PurgeInterval: -1}) // (1)!
	return gashe.New(backend, gashe.Options{})
}

var errDown = errors.New("backend is down")

type downBackend struct{ gashe.Backend } // (2)!

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }

func load(context.Context) (string, error) { return "Ann", nil }

func TestGetOrLoad(t *testing.T) {
	users := gashe.For(newCache(t), gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}})
	if u, err := users.GetOrLoad(context.Background(), "42", load); u != "Ann" || err != nil {
		t.Fatal(u, err)
	}
}

func TestBackendDown(t *testing.T) {
	c := gashe.New(downBackend{memory.New(memory.Options{PurgeInterval: -1})}, gashe.Options{})
	users := gashe.For(c, gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}})
	if u, err := users.GetOrLoad(context.Background(), "42", load); u != "Ann" || err != nil {
		t.Fatal(u, err) // the loader answers while the backend fails
	}
}
