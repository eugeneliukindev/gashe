// Package zstd is a gashe.Compressor based on Zstandard: for large values, less memory in the
// backend and less traffic, and faster than gzip for about the same size.
package zstd

import (
	"fmt"

	"github.com/klauspost/compress/zstd"

	"github.com/eugeneliukindev/gashe"
)

// Compressor compresses with Zstandard. Create it with New and share it; it is safe for concurrent
// use.
type Compressor struct {
	enc *zstd.Encoder
	dec *zstd.Decoder
}

var _ gashe.Compressor = (*Compressor)(nil)

// New returns a Compressor.
func New() *Compressor {
	// Both only fail on invalid options, and these are fixed.
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		panic(fmt.Sprintf("gashe/zstd: %v", err))
	}
	dec, err := zstd.NewReader(nil)
	if err != nil {
		panic(fmt.Sprintf("gashe/zstd: %v", err))
	}
	return &Compressor{enc: enc, dec: dec}
}

// Compress implements gashe.Compressor.
func (c *Compressor) Compress(data []byte) ([]byte, error) {
	return c.enc.EncodeAll(data, nil), nil
}

// Decompress implements gashe.Compressor.
func (c *Compressor) Decompress(data []byte) ([]byte, error) {
	raw, err := c.dec.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("gashe/zstd: decompress: %w", err)
	}
	return raw, nil
}
