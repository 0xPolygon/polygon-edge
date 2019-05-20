package discv4

import (
	"fmt"
	"net"
	"time"
)

// Based on https://github.com/hashicorp/memberlist/blob/master/mock_transport.go

// MockNetwork mocks a network of peers
type MockNetwork struct {
	transports map[string]*MockTransport
	port       int
}

// NewTransport creates a new mockup transport
func (m *MockNetwork) NewTransport() Transport {
	m.port++
	addr := &net.UDPAddr{IP: net.IP("127.0.0.1"), Port: int(m.port)}

	t := &MockTransport{
		net:      m,
		addr:     addr,
		packetCh: make(chan *Packet),
	}
	return t
}

// MockTransport mocks a udp transport
type MockTransport struct {
	net      *MockNetwork
	addr     *net.UDPAddr
	packetCh chan *Packet
}

// PacketCh implements the transport interface
func (m *MockTransport) PacketCh() chan *Packet {
	return m.packetCh
}

// WriteTo implements the transport interface
func (m *MockTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	dest, ok := m.net.transports[addr]
	if !ok {
		return time.Time{}, fmt.Errorf("No route to %q", addr)
	}

	now := time.Now()
	dest.packetCh <- &Packet{
		Buf:       b,
		From:      m.addr,
		Timestamp: now,
	}
	return now, nil
}

// Shutdown implements the transport interface
func (m *MockTransport) Shutdown() {}
