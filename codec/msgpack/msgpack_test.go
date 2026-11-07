package msgpack_test

import (
	"context"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/codec/msgpack"
)

type user struct {
	ID      int
	Name    string
	Created time.Time
}

func TestGetOrLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	c := gashe.New(mem, gashe.Options{Codec: msgpack.Codec{}})
	want := user{ID: 1, Name: "Ann", Created: time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)}
	o := gashe.Policy[user]{TTL: gashe.TTL{For: time.Minute}, Tags: []string{"users"}}

	for range 2 { // a miss, then a hit decoded from MessagePack
		got, err := gashe.GetOrLoad(ctx, c, "u:1", o, func(context.Context) (user, error) { return want, nil })
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != want.ID || got.Name != want.Name || !got.Created.Equal(want.Created) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	}
	raw, err := mem.Get(ctx, "u:1")
	if err != nil {
		t.Fatal(err)
	}
	if raw[0] == '{' {
		t.Fatalf("stored as JSON: %q", raw)
	}
}

func TestUnmarshalWrapsErrors(t *testing.T) {
	var u user
	if err := (msgpack.Codec{}).Unmarshal([]byte{0xc1}, &u); err == nil {
		t.Fatal("no error on invalid input")
	}
}
