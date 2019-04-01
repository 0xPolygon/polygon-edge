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
