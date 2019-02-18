package rlpx

import (
	"fmt"
	"io"
	"strings"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/minimal/helper/enode"
)

const (
	BaseProtocolVersion    = 5
	BaseProtocolLength     = uint64(16)
	baseProtocolMaxMsgSize = 2 * 1024

	snappyProtocolVersion = 5
)

type DiscReason uint

const (
	DiscRequested DiscReason = iota
	DiscNetworkError
	DiscProtocolError
	DiscUselessPeer
	DiscTooManyPeers
	DiscAlreadyConnected
	DiscIncompatibleVersion
	DiscInvalidIdentity
	DiscQuitting
	DiscUnexpectedIdentity
	DiscSelf
	DiscReadTimeout
	DiscSubprotocolError = 0x10
	DiscUnknown          = 0x100
)

func (d DiscReason) String() string {
	switch d {
	case DiscRequested:
		return "disconnect requested"
	case DiscNetworkError:
		return "network error"
	case DiscProtocolError:
		return "breach of protocol"
	case DiscUselessPeer:
		return "useless peer"
	case DiscTooManyPeers:
		return "too many peers"
	case DiscAlreadyConnected:
		return "already connected"
	case DiscIncompatibleVersion:
		return "incompatible p2p protocol version"
	case DiscInvalidIdentity:
		return "invalid node identity"
	case DiscQuitting:
		return "client quitting"
	case DiscUnexpectedIdentity:
		return "unexpected identity"
	case DiscSelf:
		return "connected to self"
	case DiscReadTimeout:
		return "read timeout"
	case DiscSubprotocolError:
		return "subprotocol error"
	default:
		panic(fmt.Errorf("Disc reason %d not found", d))
	}
}

func (d DiscReason) Error() string {
	return d.String()
}

func decodeDiscMsg(msg io.Reader) DiscReason {
	var reason [1]DiscReason
	if err := rlp.Decode(msg, &reason); err != nil {
		return DiscUnknown
	}
	return reason[0]
}

const (
	// devp2p message codes
	handshakeMsg = 0x00
	discMsg      = 0x01
	pingMsg      = 0x02
	pongMsg      = 0x03
)

// Cap is the peer capability.
type Cap struct {
	Name    string
	Version uint
}

func (c *Cap) less(cc *Cap) bool {
	if cmp := strings.Compare(c.Name, cc.Name); cmp != 0 {
		return cmp == -1
	}
	return c.Version < cc.Version
}

// Capabilities are all the capabilities of the other peer
type Capabilities []*Cap

func (c Capabilities) Len() int {
	return len(c)
}

func (c Capabilities) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c Capabilities) Less(i, j int) bool {
	return c[i].less(c[j])
}

// Info is the info of the node
type Info struct {
	Version    uint64
	Name       string
	Caps       Capabilities
	ListenPort uint64
	ID         enode.ID

	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}
