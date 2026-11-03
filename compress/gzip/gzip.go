// Package gzip is a gashe.Compressor based on gzip, from the standard library only: for large
// values, less memory in the backend and less traffic. compress/zstd stores about the same and is
// faster.
package gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sync"

	"github.com/eugeneliukindev/gashe"
)

// Compressor compresses with gzip. Create it with New and share it; it is safe for concurrent use
// and reuses its gzip writers and readers.
type Compressor struct {
	writers sync.Pool // *gzip.Writer
	readers sync.Pool // *gzip.Reader
}

var _ gashe.Compressor = (*Compressor)(nil)

// New returns a Compressor.
func New() *Compressor { return &Compressor{} }

// Compress implements gashe.Compressor.
func (c *Compressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, ok := c.writers.Get().(*gzip.Writer)
	if ok {
		w.Reset(&buf)
	} else {
		w = gzip.NewWriter(&buf)
	}
	defer c.writers.Put(w)
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("gashe/gzip: compress: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gashe/gzip: compress: %w", err)
	}
	return buf.Bytes(), nil
}

// Decompress implements gashe.Compressor.
func (c *Compressor) Decompress(data []byte) ([]byte, error) {
	r, ok := c.readers.Get().(*gzip.Reader)
	var err error
	if ok {
		err = r.Reset(bytes.NewReader(data))
	} else {
		r, err = gzip.NewReader(bytes.NewReader(data))
	}
	if err != nil {
		return nil, fmt.Errorf("gashe/gzip: decompress: %w", err)
	}
	defer c.readers.Put(r)
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gashe/gzip: decompress: %w", err)
	}
	return raw, nil
}
