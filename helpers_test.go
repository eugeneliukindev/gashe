package gashe_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/memory"
)

var (
	errBackendDown = errors.New("backend down")
	errAPI         = errors.New("api down")
)

type user struct {
	Name string
}

func newCache(t *testing.T) *gashe.Cache {
	t.Helper()
	mem := memory.New(memory.Options{CheckInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	return gashe.New(mem, gashe.Options{})
}

// versions returns a loader that answers 1, 2, ... and counts its runs.
func versions() (*atomic.Int32, func(context.Context) (int, error)) {
	var n atomic.Int32
	return &n, func(context.Context) (int, error) { return int(n.Add(1)), nil }
}
