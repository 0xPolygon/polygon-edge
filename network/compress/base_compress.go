package compress

import (
	"github.com/libp2p/go-libp2p-core/network"
)

// NoStreamCompressor is the basic uncompressed stream compressor
type NoStreamCompressor struct {
	BaseStreamCompressor
}

func compressNoStream(stream network.Stream) network.Stream {
	return stream
}

func (n *NoStreamCompressor) Write(b []byte) (int, error) {
	return n.Stream.Write(b)
}

func (n *NoStreamCompressor) Read(b []byte) (int, error) {
	return n.Stream.Read(b)
}

func (n *NoStreamCompressor) Close() error {
	return n.Stream.Close()
}
