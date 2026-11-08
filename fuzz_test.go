package gashe_test

import (
	"context"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// Bytes stored under a key by anyone, an older version or another service: GetOrLoad answers
// or returns an error, and never panics.
func FuzzGetOrLoadStoredBytes(f *testing.F) {
	f.Add([]byte(`{"v":{"ID":1,"Name":"Ann"}}`))
	f.Add([]byte(`{"v":null}`))
	f.Add([]byte(`not json`))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		ctx := context.Background()
		mem := memory.New(memory.Options{})
		defer mem.Close()
		if err := mem.Set(ctx, "user:1", raw, time.Minute); err != nil {
			t.Fatal(err)
		}
		users := gashe.For(gashe.New(mem, gashe.Options{}), gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}})
		_, _ = users.GetOrLoad(ctx, "user:1", func(context.Context) (User, error) {
			return User{ID: 1, Name: "Ann"}, nil
		})
	})
}
