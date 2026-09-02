package timeline

import (
	"bytes"

	"github.com/klauspost/compress/zstd"
)

// zstdEncoder and zstdDecoder are lazily initialized package-level instances
// because the klauspost zstd package builds encoder/decoder dictionaries once
// and reuses them across calls.
var (
	zstdEncoder *zstd.Encoder
	zstdDecoder *zstd.Decoder
)

// CompressPayload compresses data using zstd. An empty input returns empty.
func CompressPayload(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	enc := zstdEncoder
	if enc == nil {
		var err error
		enc, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return nil, err
		}
		zstdEncoder = enc
	}
	return enc.EncodeAll(data, nil), nil
}

// DecompressPayload decompresses zstd data.
func DecompressPayload(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	dec := zstdDecoder
	if dec == nil {
		var err error
		dec, err = zstd.NewReader(nil)
		if err != nil {
			return nil, err
		}
		zstdDecoder = dec
	}
	out, err := dec.DecodeAll(data, nil)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// compressIfNeeded compresses payload only when the compression level is not
// zero and the result is smaller than the input.
func compressIfNeeded(payload []byte, compression byte) ([]byte, byte, error) {
	if compression == 0 {
		return payload, 0, nil
	}
	compressed, err := CompressPayload(payload)
	if err != nil {
		return nil, 0, err
	}
	if len(compressed) >= len(payload) {
		return payload, 0, nil
	}
	return compressed, compression, nil
}

// decompressIfNeeded decompresses payload when a compression level is set.
func decompressIfNeeded(payload []byte, compression byte) ([]byte, error) {
	if compression == 0 {
		return payload, nil
	}
	return DecompressPayload(payload)
}

var _ = bytes.Compare
