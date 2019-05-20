package discv4

import (
	"net"
	"sync/atomic"
	"time"

	log "github.com/hashicorp/go-hclog"
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
	PacketCh() chan *Packet
	WriteTo(b []byte, addr string) (time.Time, error)
	Shutdown()
}

// UDPTransport implements the UDP Transport
type UDPTransport struct {
	logger   log.Logger
	packetCh chan *Packet
	listener *net.UDPConn
	shutdown int32
}

func newUDPTransport(udpAddr *net.UDPAddr) (Transport, error) {
	listener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, nil
	}

	t := &UDPTransport{
		listener: listener,
		packetCh: make(chan *Packet),
	}
	go t.listen()
	return t, nil
}

func (u *UDPTransport) listen() {
	for {
		buf := make([]byte, udpPacketBufSize)

		n, addr, err := u.listener.ReadFrom(buf)
		ts := time.Now()
		if err != nil {
			if s := atomic.LoadInt32(&u.shutdown); s == 1 {
				break
			}
			u.logger.Info("Error reading UDP packet", "err", err)
			continue
		}
		if n < 1 {
			u.logger.Info("UDP packet too short", "len", len(buf), "addr", addr)
			continue
		}

		u.packetCh <- &Packet{
			Buf:       buf[:n],
			From:      addr,
			Timestamp: ts,
		}
	}
}

// PacketCh implements the transport interface
func (u *UDPTransport) PacketCh() chan *Packet {
	return u.packetCh
}

// WriteTo implements the transport interface
func (u *UDPTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return time.Time{}, err
	}
	_, err = u.listener.WriteTo(b, udpAddr)
	return time.Now(), err
}

// Shutdown implements the transport interface
func (u *UDPTransport) Shutdown() {
	atomic.StoreInt32(&u.shutdown, 1)
	u.listener.Close()
}
