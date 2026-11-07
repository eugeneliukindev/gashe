// Package contract checks a backend of your own against the contract of gashe.Backend.
package contract

import (
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/backendtest"
)

// newEmptyBackend stands for your own backend.
func newEmptyBackend(*testing.T) gashe.Backend {
	return memory.New(memory.Options{PurgeInterval: -1})
}

func TestContract(t *testing.T) {
	backendtest.Run(t, func(t *testing.T) gashe.Backend {
		return newEmptyBackend(t) // (1)!
	})
}
