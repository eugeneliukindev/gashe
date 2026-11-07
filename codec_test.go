package gashe_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

// marked is a Compressor that puts a mark in front, to see that every path goes through it.
type marked struct{}

func (marked) Compress(data []byte) ([]byte, error) { return append([]byte("z:"), data...), nil }

func (marked) Decompress(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte("z:")) {
		return nil, errors.New("not compressed")
	}
	return data[2:], nil
}

func TestCompressorOnEveryPath(t *testing.T) {
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	c := gashe.New(mem, gashe.Options{Compressor: marked{}})

	if err := c.Set(ctx, "a", "Ann", 0); err != nil {
		t.Fatal(err)
	}
	_, _ = gashe.GetOrLoad(ctx, c, "b", gashe.Policy[string]{TTL: gashe.TTL{For: time.Hour}},
		func(context.Context) (string, error) { return "Bob", nil })
	for _, key := range []string{"a", "b"} {
		if raw, _ := mem.Get(ctx, key); !bytes.HasPrefix(raw, []byte("z:")) {
			t.Fatalf("%s stored %q without the compressor", key, raw)
		}
	}
	var got string
	if err := c.Get(ctx, "a", &got); err != nil || got != "Ann" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	values, err := c.GetMany(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if err := values[0].Decode(&got); err != nil || got != "Ann" {
		t.Fatalf("GetMany = %q, %v", got, err)
	}
}
