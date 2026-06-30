// Package jsonx wraps github.com/bytedance/sonic for swl JSON I/O.
package jsonx

import (
	"io"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/decoder"
)

// Config is the frozen sonic API used across swl-go (fast path, int64-friendly).
var Config = sonic.Config{
	CopyString: true,
	UseInt64:   true,
}.Froze()

// Marshal serializes v to JSON.
func Marshal(v any) ([]byte, error) {
	return Config.Marshal(v)
}

// Unmarshal parses JSON into v.
func Unmarshal(data []byte, v any) error {
	return Config.Unmarshal(data, v)
}

// NewStreamDecoder returns a streaming decoder (array of objects, etc.).
func NewStreamDecoder(r io.Reader) *decoder.StreamDecoder {
	return decoder.NewStreamDecoder(r)
}
