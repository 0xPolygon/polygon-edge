package compress

import (
	"compress/gzip"
	"github.com/libp2p/go-libp2p-core/network"
	"sync"
)

// GzipStreamCompressor is the gzip stream compression implementation
type GzipStreamCompressor struct {
	BaseStreamCompressor

	sync.Mutex

	writer *gzip.Writer
	reader *gzip.Reader
}

func compressGzipStream(stream network.Stream) network.Stream {
	return &GzipStreamCompressor{
		BaseStreamCompressor: BaseStreamCompressor{
			Stream: stream,
		},
		writer: gzip.NewWriter(stream),
	}
}

func (g *GzipStreamCompressor) Write(b []byte) (int, error) {
	g.Lock()
	defer g.Unlock()

	n, err := g.writer.Write(b)
	if err != nil {
		return n, err
	}

	return n, g.writer.Flush()
}

func (g *GzipStreamCompressor) Read(b []byte) (int, error) {
	if err := g.initReader(); err != nil {
		return 0, err
	}

	n, err := g.reader.Read(b)
	if err != nil {
		_ = g.reader.Close()
	}

	return n, err
}

func (g *GzipStreamCompressor) initReader() error {
	if g.reader != nil {
		return nil
	}

	var err error
	g.reader, err = gzip.NewReader(g.Stream)

	return err
}

func (g *GzipStreamCompressor) Close() error {
	g.Lock()
	defer g.Unlock()

	if err := g.writer.Close(); err != nil {
		return err
	}

	return g.Stream.Close()
}
