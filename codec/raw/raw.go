// Package raw is a gashe.Codec that stores []byte and string values as they are, without the
// quotes and base64 of JSON, for pages, images or payloads that are already encoded.
package raw

import (
	"bytes"

	"github.com/eugeneliukindev/gashe"
)

// Codec stores []byte and string values as they are; other values go to Inner, and a nil Inner
// means gashe.JSON. GetOrLoad stores its result inside an entry of its own, so Codec helps only Get,
// Set and the other calls of the basic API.
type Codec struct {
	Inner gashe.Codec
}

var _ gashe.Codec = Codec{}

// Marshal implements gashe.Codec.
func (c Codec) Marshal(v any) ([]byte, error) {
	switch v := v.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	}
	if c.Inner == nil {
		return gashe.JSON{}.Marshal(v)
	}
	return c.Inner.Marshal(v)
}

// Unmarshal implements gashe.Codec.
func (c Codec) Unmarshal(data []byte, v any) error {
	switch v := v.(type) {
	case *[]byte:
		*v = bytes.Clone(data)
		return nil
	case *string:
		*v = string(data)
		return nil
	}
	if c.Inner == nil {
		return gashe.JSON{}.Unmarshal(data, v)
	}
	return c.Inner.Unmarshal(data, v)
}
