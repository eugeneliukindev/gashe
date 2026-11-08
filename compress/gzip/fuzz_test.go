package gzip_test

import (
	"bytes"
	"testing"

	"github.com/eugeneliukindev/gashe/compress/gzip"
)

// What Compress gives back decompresses to the same bytes, and Decompress of any bytes returns
// an error instead of panicking.
func FuzzCompressor(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("abcabcabcabc: repeats compress"))
	f.Fuzz(func(t *testing.T, data []byte) {
		c := gzip.New()
		packed, err := c.Compress(data)
		if err != nil {
			t.Fatal(err)
		}
		got, err := c.Decompress(packed)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("Decompress(Compress(%q)) = %q, %v", data, got, err)
		}
		_, _ = c.Decompress(data)
	})
}
