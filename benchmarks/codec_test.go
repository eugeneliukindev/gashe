package benchmarks_test

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/codec/msgpack"
	"github.com/eugeneliukindev/gashe/compress/gzip"
	"github.com/eugeneliukindev/gashe/compress/zstd"
)

type user struct {
	ID    int
	Name  string
	Email string
	Admin bool
}

type product struct {
	ID          int
	Name        string
	Description string
	Price       float64
	Tags        []string
}

var words = []string{
	"red",
	"blue",
	"steel",
	"wooden",
	"chair",
	"table",
	"lamp",
	"large",
	"small",
	"modern",
	"classic",
	"set",
}

// newProduct returns a product with a description of n words, the same for the same seed.
func newProduct(r *rand.Rand, id, n int) product {
	var d strings.Builder
	for range n {
		d.WriteString(words[r.IntN(len(words))] + " ")
	}
	return product{
		ID:          id,
		Name:        fmt.Sprintf("product %d %s", id, words[r.IntN(len(words))]),
		Description: d.String(),
		Price:       r.Float64() * 100,
		Tags:        []string{words[r.IntN(len(words))], words[r.IntN(len(words))]},
	}
}

// values are the three sizes a cache usually holds: one small record, one item with text, a list.
func values() []struct {
	name string
	v    any
	new  func() any // a pointer to decode into
} {
	r := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // fixed test data, not a secret
	list := make([]product, 500)
	for i := range list {
		list[i] = newProduct(r, i, 20)
	}
	return []struct {
		name string
		v    any
		new  func() any
	}{
		{"user", user{ID: 42, Name: "Ann Smith", Email: "ann@example.com"}, func() any { return new(user) }},
		{"product", newProduct(r, 1, 150), func() any { return new(product) }},
		{"list", list, func() any { return new([]product) }},
	}
}

// codecs are the formats; compressors are what may shrink them. A Cache runs the codec, then the
// compressor.
var (
	codecs = []struct {
		name  string
		codec gashe.Codec
	}{
		{"json", gashe.JSON{}},
		{"msgpack", msgpack.Codec{}},
	}
	compressors = []struct {
		name       string
		compressor gashe.Compressor
	}{
		{"none", nil},
		{"gzip", gzip.New()},
		{"zstd", zstd.New()},
	}
)

// encode and decode do what a Cache does with Options.Codec and Options.Compressor.
func encode(c gashe.Codec, z gashe.Compressor, v any) ([]byte, error) {
	data, err := c.Marshal(v)
	if err != nil || z == nil {
		return data, err
	}
	return z.Compress(data)
}

func decode(c gashe.Codec, z gashe.Compressor, data []byte, v any) error {
	if z != nil {
		var err error
		if data, err = z.Decompress(data); err != nil {
			return err
		}
	}
	return c.Unmarshal(data, v)
}

// BenchmarkCodec encodes and decodes every value with every codec and compressor, and reports the
// stored size.
func BenchmarkCodec(b *testing.B) {
	for _, val := range values() {
		for _, c := range codecs {
			for _, z := range compressors {
				data, err := encode(c.codec, z.compressor, val.v)
				if err != nil {
					b.Fatal(err)
				}
				name := val.name + "/" + c.name + "+" + z.name
				b.Run(name+"/encode", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, err := encode(c.codec, z.compressor, val.v); err != nil {
							b.Fatal(err)
						}
					}
					b.ReportMetric(float64(len(data)), "bytes")
				})
				b.Run(name+"/decode", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if err := decode(c.codec, z.compressor, data, val.new()); err != nil {
							b.Fatal(err)
						}
					}
					b.ReportMetric(float64(len(data)), "bytes")
				})
			}
		}
	}
}
