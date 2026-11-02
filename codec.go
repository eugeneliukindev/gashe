package gashe

import "encoding/json"

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
