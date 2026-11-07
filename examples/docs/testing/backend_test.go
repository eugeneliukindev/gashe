//go:build go1.25

package users

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backendtest"
)

// loggingBackend is your own backend; here it wraps memory to count reads.
type loggingBackend struct {
	gashe.Backend
	reads atomic.Int64 // (1)!
}

func (b *loggingBackend) Get(ctx context.Context, key string) ([]byte, error) {
	b.reads.Add(1)
	return b.Backend.Get(ctx, key)
}

func TestLoggingBackendKeepsTheContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return &loggingBackend{Backend: memory.New(memory.Options{PurgeInterval: -1})} // empty for every subtest
	})
}
