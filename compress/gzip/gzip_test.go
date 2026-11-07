package gzip_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/compress/gzip"
)

type page struct {
	Body string
}

func TestGetOrLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	c := gashe.New(mem, gashe.Options{Compressor: gzip.New()})
	want := page{Body: strings.Repeat("<li>a product</li>", 10_000)}

	for range 2 { // a miss, then a hit read back through a pooled reader
		got, err := gashe.GetOrLoad(ctx, c, "p", gashe.Policy[page]{TTL: gashe.TTL{For: time.Minute}},
			func(context.Context) (page, error) { return want, nil })
		if err != nil || got != want {
			t.Fatalf("got %d bytes, %v; want the page back", len(got.Body), err)
		}
	}
	raw, err := mem.Get(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte{0x1f, 0x8b}) || len(raw) > len(want.Body)/10 {
		t.Fatalf("stored %d bytes, not a small gzip stream", len(raw))
	}
}

func TestDecompressWrapsErrors(t *testing.T) {
	_, err := gzip.New().Decompress([]byte("not gzip"))
	if err == nil || !strings.HasPrefix(err.Error(), "gashe/gzip: ") {
		t.Fatalf("err = %v, want a gashe/gzip error", err)
	}
}

func TestConcurrentUse(t *testing.T) {
	c := gzip.New()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			want := []byte(strings.Repeat("n", i+1))
			for range 50 {
				data, err := c.Compress(want)
				var got []byte
				if err == nil {
					got, err = c.Decompress(data)
				}
				if err != nil || !bytes.Equal(got, want) {
					t.Errorf("goroutine %d: %q, %v", i, got, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
