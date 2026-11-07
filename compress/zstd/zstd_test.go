package zstd_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/compress/zstd"
)

type page struct {
	Title string
	Body  string
}

func TestGetOrLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	c := gashe.New(mem, gashe.Options{Compressor: zstd.New()})
	want := page{Title: "products", Body: strings.Repeat("<li>a product</li>", 10_000)}
	o := gashe.Policy[page]{TTL: gashe.TTL{For: time.Minute}, Tags: []string{"pages"}}

	for range 2 { // a miss, then a hit decoded from the compressed entry
		got, err := gashe.GetOrLoad(ctx, c, "p", o, func(context.Context) (page, error) { return want, nil })
		if err != nil || got != want {
			t.Fatalf("got %d bytes, %v; want the page back", len(got.Body), err)
		}
	}
	raw, err := mem.Get(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		t.Fatalf("not a zstd frame: %q", raw[:min(len(raw), 8)])
	}
	if len(raw) > len(want.Body)/10 {
		t.Fatalf("stored %d bytes for a body of %d; want it compressed", len(raw), len(want.Body))
	}
}

func TestDecompressWrapsErrors(t *testing.T) {
	_, err := zstd.New().Decompress([]byte("not zstd"))
	if err == nil || !strings.HasPrefix(err.Error(), "gashe/zstd: ") {
		t.Fatalf("err = %v, want a gashe/zstd error", err)
	}
}

func TestConcurrentUse(t *testing.T) {
	c := zstd.New()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			want := []byte(strings.Repeat("n", i+1))
			data, err := c.Compress(want)
			var got []byte
			if err == nil {
				got, err = c.Decompress(data)
			}
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("goroutine %d: %q, %v", i, got, err)
			}
		})
	}
	wg.Wait()
}
