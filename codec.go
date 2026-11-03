package gashe

import "encoding/json"

// Codec turns a value into bytes and back: the format of what is stored.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// Compressor shrinks the bytes a Codec produced and restores them. It knows nothing of values; the
// packages under compress/ provide gzip and Zstandard.
type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
}

// noCompression is the Compressor of a Cache without Options.Compressor.
type noCompression struct{}

func (noCompression) Compress(data []byte) ([]byte, error)   { return data, nil }
func (noCompression) Decompress(data []byte) ([]byte, error) { return data, nil }

// compressed is the Codec a Cache uses: the format of Options.Codec, then the compression of
// Options.Compressor.
type compressed struct {
	codec      Codec
	compressor Compressor
}

func (c compressed) Marshal(v any) ([]byte, error) {
	raw, err := c.codec.Marshal(v)
	if err != nil {
		return nil, err
	}
	return c.compressor.Compress(raw)
}

func (c compressed) Unmarshal(data []byte, v any) error {
	raw, err := c.compressor.Decompress(data)
	if err != nil {
		return err
	}
	return c.codec.Unmarshal(raw, v)
}

// JSON is the default Codec, based on encoding/json.
type JSON struct{}

// Marshal implements Codec.
func (JSON) Marshal(v any) ([]byte, error) { return json.Marshal(v) }

// Unmarshal implements Codec.
func (JSON) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
