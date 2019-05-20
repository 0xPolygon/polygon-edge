package discv4

import (
	"net"
	"time"
)

const (
	// udpPacketBufSize is used to buffer incoming packets during read
	// operations.
	udpPacketBufSize = 1280
)

// Packet is used to provide some metadata about incoming UDP packets from peers
type Packet struct {
	Buf       []byte
	From      net.Addr
	Timestamp time.Time
}

// Transport is the transport used by discv4
type Transport interface {
	// Addr returns the bind address of the transport
	Addr() *net.UDPAddr

	// PacketCh returns a channel to receive incomming packets
	PacketCh() chan *Packet

	// WriteTo writes the payload to the given address
	WriteTo(b []byte, addr string) (time.Time, error)

	// Shutdown closes the listener
	Shutdown()
}
