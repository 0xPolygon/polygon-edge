package compress

import (
	"errors"
	"github.com/libp2p/go-libp2p-core/network"
)

var (
	errInvalidCompressor = errors.New("invalid stream compressor type")
)

type CompressStream func(stream network.Stream) network.Stream

// BaseStreamCompressor is the base stream compressor type
type BaseStreamCompressor struct {
	network.Stream
}

type StreamCompressorType string

const (
	GzipCompressor StreamCompressorType = "gzip"
	NoCompressor   StreamCompressorType = "uncompressed"
)

var (
	compressorBackends = map[StreamCompressorType]CompressStream{
		GzipCompressor: compressGzipStream,
		NoCompressor:   compressNoStream,
	}
)

// GetStreamCompressor fetches the specified stream compressor. If it's not found
// an appropriate error is returned
func GetStreamCompressor(compressorType StreamCompressorType) (CompressStream, error) {
	compressFunc, found := compressorBackends[compressorType]
	if !found {
		return nil, errInvalidCompressor
	}

	return compressFunc, nil
}
