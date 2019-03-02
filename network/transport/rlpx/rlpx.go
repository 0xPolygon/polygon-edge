package rlpx

import (
	"crypto/ecdsa"
	"errors"
	"net"

	"github.com/umbracle/minimal/helper/enode"
)

// Server returns a new Rlpx server side Session
func Server(conn net.Conn, prv *ecdsa.PrivateKey, info *Info) *Session {
	return &Session{conn: conn, prv: prv, Info: info}
}

// Client returns a new Rlpx client side Session
func Client(conn net.Conn, prv *ecdsa.PrivateKey, pub *ecdsa.PublicKey, info *Info) *Session {
	return &Session{conn: conn, prv: prv, pub: pub, Info: info, isClient: true}
}

// Listener implements a network listener for Rlpx sessions.
type Listener struct {
	net.Listener
	config *Config
}

// Accept waits for and returns the next Session to the listener.
func (l *Listener) Accept() (*Session, error) {
	rawConn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	conn := Server(rawConn, l.config.Prv, l.config.Info)
	if err := conn.Handshake(); err != nil {
		rawConn.Close()
		return nil, err
	}
	return conn, nil
}

// NewListener creates a Listener which accepts Rlpx Sessions
func NewListener(inner net.Listener, config *Config) *Listener {
	l := new(Listener)
	l.Listener = inner
	l.config = config
	return l
}

// Listen creates a Rlpx listener accepting Sessions on the
// given network address using net.Listen.
func Listen(network, laddr string, config *Config) (*Listener, error) {
	if config == nil || (config.Prv == nil) {
		return nil, errors.New("rlpx: private key not set")
	}
	l, err := net.Listen(network, laddr)
	if err != nil {
		return nil, err
	}
	return NewListener(l, config), nil
}

// DialWithDialer connects to the given network address using dialer.Dial and
// then initiates a Rlpx handshake
func DialWithDialer(dialer *net.Dialer, network, addr string, config *Config) (*Session, error) {
	rawConn, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	conn := Client(rawConn, config.Prv, config.Pub, config.Info)
	if err := conn.Handshake(); err != nil {
		rawConn.Close()
		return conn, err
	}
	return conn, nil
}

// Dial connects to the given network address using net.Dial
// and then initiates a Rlpx handshake.
func Dial(network, addr string, config *Config) (*Session, error) {
	return DialWithDialer(new(net.Dialer), network, addr, config)
}

// DialEnode connects to the given enode address using net.Dial
// and then initiates a Rlpx handshake.
func DialEnode(network, addr string, config *Config) (*Session, error) {
	enode, err := enode.ParseURL(addr)
	if err != nil {
		return nil, err
	}
	pub, err := enode.PublicKey()
	if err != nil {
		return nil, err
	}
	tcpAddr := net.TCPAddr{IP: enode.IP, Port: int(enode.TCP)}
	return DialWithDialer(new(net.Dialer), network, tcpAddr.String(), &Config{Pub: pub, Prv: config.Prv, Info: config.Info})
}
