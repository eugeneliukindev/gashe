// Package msgpack is a gashe.Codec based on MessagePack: smaller and faster to decode than JSON.
//
// Values stored with it are binary, so Cache.IncrBy cannot read them; keep counters under keys that
// are only touched by IncrBy.
package msgpack

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// Codec is a gashe.Codec based on github.com/vmihailenco/msgpack/v5. Its zero value is ready
// to use.
type Codec struct{}

// Marshal implements gashe.Codec.
func (Codec) Marshal(v any) ([]byte, error) {
	data, err := msgpack.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("gashe/msgpack: marshal: %w", err)
	}
	return data, nil
}

// Unmarshal implements gashe.Codec.
func (Codec) Unmarshal(data []byte, v any) error {
	if err := msgpack.Unmarshal(data, v); err != nil {
		return fmt.Errorf("gashe/msgpack: unmarshal: %w", err)
	}
	return nil
}
