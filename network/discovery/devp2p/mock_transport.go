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

func newMockNetwork() *MockNetwork {
	return &MockNetwork{
		transports: map[string]*MockTransport{},
		port:       0,
	}
}

// NewTransport creates a new mockup transport
func (m *MockNetwork) NewTransport() Transport {
	m.port++
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: int(m.port)}

	t := &MockTransport{
		net:      m,
		addr:     addr,
		packetCh: make(chan *Packet, 10),
	}
	if m.transports == nil {
		m.transports = make(map[string]*MockTransport)
	}
	m.transports[addr.String()] = t
	return t
}

// MockTransport mocks a udp transport
type MockTransport struct {
	net      *MockNetwork
	addr     *net.UDPAddr
	packetCh chan *Packet
}

// Addr implements the transport interface
func (m *MockTransport) Addr() *net.UDPAddr {
	return m.addr
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
func (m *MockTransport) Shutdown() {
}
