package rlpx

import (
	"crypto/ecdsa"
	"net"

	"github.com/umbracle/minimal/helper/enode"
	"github.com/umbracle/minimal/network/common"
	"github.com/umbracle/minimal/protocol"
)

// Rlpx is the RLPx transport protocol
type Rlpx struct {
	priv     *ecdsa.PrivateKey
	backends []protocol.Backend
	info     *common.Info
}

// getProtocol returns a protocol
func (r *Rlpx) getProtocol(name string, version uint) protocol.Backend {
	for _, p := range r.backends {
		proto := p.Protocol()
		if proto.Name == name && proto.Version == version {
			return p
		}
	}
	return nil
}

// Connect implements the connect interface
func (r *Rlpx) Connect(rawConn net.Conn, enode enode.Enode) (common.Session, error) {
	pub, err := enode.PublicKey()
	if err != nil {
		return nil, err
	}

	conn := Client(r, rawConn, r.priv, pub, networkInfoToLocalInfo(r.info))
	if err := conn.Handshake(); err != nil {
		rawConn.Close()
		return conn, err
	}
	return conn, nil
}

// Accept implements the transport interface
func (r *Rlpx) Accept(rawConn net.Conn) (common.Session, error) {
	conn := Server(r, rawConn, r.priv, networkInfoToLocalInfo(r.info))
	if err := conn.Handshake(); err != nil {
		rawConn.Close()
		return nil, err
	}
	return conn, nil
}

// Setup implements the transport interface
func (r *Rlpx) Setup(priv *ecdsa.PrivateKey, backends []protocol.Backend, info *common.Info) {
	r.priv = priv
	r.backends = backends
	r.info = info
}

// Server returns a new Rlpx server side Session
func Server(rlpx *Rlpx, conn net.Conn, prv *ecdsa.PrivateKey, info *Info) *Session {
	return &Session{rlpx: rlpx, conn: conn, prv: prv, Info: info}
}

// Client returns a new Rlpx client side Session
func Client(rlpx *Rlpx, conn net.Conn, prv *ecdsa.PrivateKey, pub *ecdsa.PublicKey, info *Info) *Session {
	return &Session{rlpx: rlpx, conn: conn, prv: prv, pub: pub, Info: info, isClient: true}
}

/*
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
*/

// networkInfoToLocalInfo converts the network info message into rlpx.Info
func networkInfoToLocalInfo(info *common.Info) *Info {
	rlpxInfo := &Info{
		Version:    BaseProtocolVersion,
		Name:       info.Client,
		ListenPort: uint64(info.Enode.TCP),
		ID:         info.Enode.ID,
	}
	for _, cap := range info.Capabilities {
		p := cap.Protocol
		rlpxInfo.Caps = append(rlpxInfo.Caps, &Cap{Name: p.Name, Version: p.Version})
	}
	return rlpxInfo
}
