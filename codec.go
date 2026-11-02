package gashe

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
)

// Codec turns a value into stored bytes and back.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// JSON is the default Codec, based on encoding/json.
type JSON struct{}

// Marshal implements Codec.
func (JSON) Marshal(v any) ([]byte, error) { return json.Marshal(v) }

// Unmarshal implements Codec.
func (JSON) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

// Gzip compresses what Inner encodes, for large values: less memory in Redis and less traffic.
// A nil Inner means JSON.
type Gzip struct {
	Inner Codec
}

// Marshal implements Codec.
func (g Gzip) Marshal(v any) ([]byte, error) {
	inner := g.Inner
	if inner == nil {
		inner = JSON{}
	}
	raw, err := inner.Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	return buf.Bytes(), nil
}

// Unmarshal implements Codec.
func (g Gzip) Unmarshal(data []byte, v any) error {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	if g.Inner == nil {
		return JSON{}.Unmarshal(raw, v)
	}
	return g.Inner.Unmarshal(raw, v)
}
