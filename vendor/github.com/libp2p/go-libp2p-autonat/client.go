package autonat

import (
	"context"
	"fmt"

	pb "github.com/libp2p/go-libp2p-autonat/pb"
	"github.com/libp2p/go-libp2p-core/helpers"

	ggio "github.com/gogo/protobuf/io"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// AutoNATError is the class of errors signalled by AutoNAT services
type AutoNATError struct {
	Status pb.Message_ResponseStatus
	Text   string
}

// NewAutoNATClient creates a fresh instance of an AutoNATClient
// If addrFunc is nil, h.Addrs will be used
func NewAutoNATClient(h host.Host, addrFunc AddrFunc) Client {
	if addrFunc == nil {
		addrFunc = h.Addrs
	}
	return &client{h: h, addrFunc: addrFunc}
}

type client struct {
	h        host.Host
	addrFunc AddrFunc
}

func (c *client) DialBack(ctx context.Context, p peer.ID) (ma.Multiaddr, error) {
	s, err := c.h.NewStream(ctx, p, AutoNATProto)
	if err != nil {
		return nil, err
	}
	// Might as well just reset the stream. Once we get to this point, we
	// don't care about being nice.
	defer helpers.FullClose(s)

	r := ggio.NewDelimitedReader(s, network.MessageSizeMax)
	w := ggio.NewDelimitedWriter(s)

	req := newDialMessage(peer.AddrInfo{ID: c.h.ID(), Addrs: c.addrFunc()})
	err = w.WriteMsg(req)
	if err != nil {
		s.Reset()
		return nil, err
	}

	var res pb.Message
	err = r.ReadMsg(&res)
	if err != nil {
		s.Reset()
		return nil, err
	}

	if res.GetType() != pb.Message_DIAL_RESPONSE {
		return nil, fmt.Errorf("Unexpected response: %s", res.GetType().String())
	}

	status := res.GetDialResponse().GetStatus()
	switch status {
	case pb.Message_OK:
		addr := res.GetDialResponse().GetAddr()
		return ma.NewMultiaddrBytes(addr)

	default:
		return nil, AutoNATError{Status: status, Text: res.GetDialResponse().GetStatusText()}
	}
}

func (e AutoNATError) Error() string {
	return fmt.Sprintf("AutoNAT error: %s (%s)", e.Text, e.Status.String())
}

func (e AutoNATError) IsDialError() bool {
	return e.Status == pb.Message_E_DIAL_ERROR
}

func (e AutoNATError) IsDialRefused() bool {
	return e.Status == pb.Message_E_DIAL_REFUSED
}

// IsDialError returns true if the AutoNAT peer signalled an error dialing back
func IsDialError(e error) bool {
	ae, ok := e.(AutoNATError)
	return ok && ae.IsDialError()
}

// IsDialRefused returns true if the AutoNAT peer signalled refusal to dial back
func IsDialRefused(e error) bool {
	ae, ok := e.(AutoNATError)
	return ok && ae.IsDialRefused()
}
