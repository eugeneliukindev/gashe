package gashe_test

import (
	"context"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

type userV1 struct{ Name string }

type userV2 struct{ FullName string }

func TestChangedTypeIsAMiss(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	_, _ = gashe.GetOrLoad(ctx, c, "user:42", gashe.Policy[userV1]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (userV1, error) { return userV1{Name: "Ann"}, nil })

	// The next deploy renamed the field: the stored entry must not decode into an empty FullName.
	got, err := gashe.GetOrLoad(ctx, c, "user:42", gashe.Policy[userV2]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (userV2, error) { return userV2{FullName: "Ann"}, nil })
	if err != nil || got.FullName != "Ann" {
		t.Fatalf("got %+v, %v; want the loader result, not the old entry", got, err)
	}
}

func TestSameTypeIsAHit(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.Policy[userV1]{TTL: gashe.TTL{For: time.Hour}}
	calls := 0
	load := func(context.Context) (userV1, error) { calls++; return userV1{Name: "Ann"}, nil }
	_, _ = gashe.GetOrLoad(ctx, c, "user:42", o, load)
	_, _ = gashe.GetOrLoad(ctx, c, "user:42", o, load)
	if calls != 1 {
		t.Fatalf("loader ran %d times, want 1", calls)
	}
}

func TestEntryWithoutShapeIsAccepted(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	// An entry as v0.1.0 stored it, without "t".
	raw := []byte(`{"at":"` + time.Now().Add(time.Hour).Format(time.RFC3339Nano) + `","v":{"Name":"Ann"}}`)
	if err := c.Backend().Set(ctx, "user:42", raw, time.Hour); err != nil {
		t.Fatal(err)
	}
	got, err := gashe.GetOrLoad(ctx, c, "user:42", gashe.Policy[userV1]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (userV1, error) {
			t.Fatal("an entry without a shape must be read, so an upgrade does not empty the cache")
			return userV1{}, nil
		})
	if err != nil || got.Name != "Ann" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

type tagged struct {
	Name string `json:"name"`
}

type withPrivate struct {
	Name string
	note string //nolint:unused // only its presence matters: encoding/json does not store it
}

func TestShapeFollowsWhatJSONStores(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	store := func(key string) {
		_, _ = gashe.GetOrLoad(ctx, c, key, gashe.Policy[userV1]{TTL: gashe.TTL{For: time.Hour}},
			func(context.Context) (userV1, error) { return userV1{Name: "Ann"}, nil })
	}
	store("a")
	store("b")
	missed := false
	_, _ = gashe.GetOrLoad(ctx, c, "a", gashe.Policy[tagged]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (tagged, error) { missed = true; return tagged{}, nil })
	if !missed {
		t.Fatal("a new json tag must change the shape")
	}
	_, _ = gashe.GetOrLoad(ctx, c, "b", gashe.Policy[withPrivate]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (withPrivate, error) {
			t.Fatal("an unexported field must not change the shape")
			return withPrivate{}, nil
		})
}

type packedV1 struct {
	Name string `msgpack:"name"`
}

type packedV2 struct {
	Name string `msgpack:"full_name"`
}

// The msgpack codec names fields by their msgpack tags: a renamed tag must be a miss, or old
// entries would read with empty fields.
func TestShapeFollowsMsgpackTags(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	_, _ = gashe.GetOrLoad(ctx, c, "a", gashe.Policy[packedV1]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (packedV1, error) { return packedV1{Name: "Ann"}, nil })
	missed := false
	_, _ = gashe.GetOrLoad(ctx, c, "a", gashe.Policy[packedV2]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (packedV2, error) { missed = true; return packedV2{}, nil })
	if !missed {
		t.Fatal("a renamed msgpack tag must change the shape")
	}
}
