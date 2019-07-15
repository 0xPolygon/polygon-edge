package rlpx

import (
	"testing"

	"github.com/umbracle/minimal/network"
)

func BenchmarkSendRecv256(b *testing.B) {
	benchmarkSendRecv(b, 256)
}

func BenchmarkSendRecv512(b *testing.B) {
	benchmarkSendRecv(b, 512)
}
func BenchmarkSendRecv1024(b *testing.B) {
	benchmarkSendRecv(b, 1024)
}

func BenchmarkSendRecv2048(b *testing.B) {
	benchmarkSendRecv(b, 2048)
}

func BenchmarkSendRecv4096(b *testing.B) {
	benchmarkSendRecv(b, 4096)
}

func benchmarkSendRecv(b *testing.B, sendSize uint32) {
	client, server := pipe(nil)

	spec := network.ProtocolSpec{}
	c := client.OpenStream(20, 10, spec)
	s := server.OpenStream(20, 10, spec)

	sendBuf := make([]byte, sendSize)

	var sendHeader Header
	sendHeader = make([]byte, HeaderSize)
	sendHeader.Encode(1, uint32(len(sendBuf)))

	recvBuf := make([]byte, sendSize+HeaderSize)
	doneCh := make(chan struct{})

	b.SetBytes(int64(sendSize))
	b.ReportAllocs()
	b.ResetTimer()

	go func() {
		for i := 0; i < b.N; i++ {
			if _, err := c.Read(recvBuf); err != nil {
				b.Fatal(err)
			}
		}
		doneCh <- struct{}{}
	}()

	for i := 0; i < b.N; i++ {
		if _, err := s.Write(sendHeader); err != nil {
			b.Fatal("")
		}
		if _, err := s.Write(sendBuf); err != nil {
			b.Fatal("")
		}
	}
	<-doneCh
}
