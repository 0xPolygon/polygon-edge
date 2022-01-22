package enode

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strconv"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
)

const nodeIDBytes = 512 / 8

// ID is the unique identifier of each node.
type ID [nodeIDBytes]byte

func (i ID) String() string {
	return hex.EncodeToString(i[:])
}

// Enode is the URL scheme description of an ethereum node.
type Enode struct {
	ID  ID
	TCP uint16
	UDP uint16
	IP  net.IP
}

// ParseURL parses an enode address
func ParseURL(rawurl string) (*Enode, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "enode" {
		return nil, fmt.Errorf("invalid URL scheme, expected 'enode'")
	}

	var id ID

	h, err := hex.DecodeString(u.User.String())
	if err != nil {
		return nil, fmt.Errorf("failed to decode id: %w", err)
	}

	if len(h) != nodeIDBytes {
		return nil, fmt.Errorf("id not found")
	}

	copy(id[:], h)

	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid host: %w", err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address '%s'", host)
	}

	tcpPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid tcp port '%s': %w", port, err)
	}

	udpPort := tcpPort
	if discPort := u.Query().Get("discport"); discPort != "" {
		udpPort, err = strconv.ParseUint(discPort, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid udp port '%s': %w", discPort, err)
		}
	}

	node := &Enode{
		ID:  id,
		TCP: uint16(tcpPort),
		UDP: uint16(udpPort),
		IP:  ip,
	}

	return node, nil
}

func (n *Enode) String() string {
	url := fmt.Sprintf("enode://%s@%s", n.ID.String(), (&net.TCPAddr{IP: n.IP, Port: int(n.TCP)}).String())

	if n.TCP != n.UDP {
		url += "?discport=" + strconv.Itoa(int(n.UDP))
	}

	return url
}

// PublicKey returns the public key of the enode
func (n *Enode) PublicKey() (*ecdsa.PublicKey, error) {
	return NodeIDToPubKey(n.ID[:])
}

// TCPAddr returns the TCP address
func (n *Enode) TCPAddr() net.TCPAddr {
	return net.TCPAddr{IP: n.IP, Port: int(n.TCP)}
}

// NodeIDToPubKey returns the public key of the enode ID
func NodeIDToPubKey(buf []byte) (*ecdsa.PublicKey, error) {
	if len(buf) != nodeIDBytes {
		return nil, fmt.Errorf("not enough length: expected %d but found %d", nodeIDBytes, len(buf))
	}

	p := &ecdsa.PublicKey{Curve: crypto.S256, X: new(big.Int), Y: new(big.Int)}
	half := len(buf) / 2
	p.X.SetBytes(buf[:half])
	p.Y.SetBytes(buf[half:])

	if !p.Curve.IsOnCurve(p.X, p.Y) {
		return nil, errors.New("id is invalid secp256k1 curve point")
	}

	return p, nil
}

// PubkeyToEnode converts a public key to an enode
func PubkeyToEnode(pub *ecdsa.PublicKey) ID {
	var id ID

	pbytes := crypto.MarshalPublicKey(pub)
	copy(id[:], pbytes[1:])

	return id
}
