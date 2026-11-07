package users

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

var errDown = errors.New("redis: connection refused")

// downBackend is a backend that is down: embed a real one, override what must fail.
type downBackend struct{ gashe.Backend }

func (downBackend) Get(context.Context, string) ([]byte, error) { return nil, errDown }
func (downBackend) GetMany(context.Context, ...string) ([][]byte, error) {
	return nil, errDown
}
func (downBackend) Set(context.Context, string, []byte, time.Duration) error { return errDown }

func TestGetWorksWhileTheCacheIsDown(t *testing.T) {
	queries := 0
	users := New(newCache(t, downBackend{memory.New(memory.Options{PurgeInterval: -1})}), countingQuery(&queries))
	u, err := users.Get(t.Context(), "42")
	if err != nil || u.Name != "user 42" {
		t.Fatalf("Get = %v, %v; want the user from the database", u, err)
	}
}
