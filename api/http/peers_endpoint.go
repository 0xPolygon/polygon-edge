package http

import (
	"fmt"

	"github.com/umbracle/minimal/network"
	"github.com/valyala/fasthttp"
)

// PeersList returns a list of peers
func (h *HTTP) PeersList(ctx *fasthttp.RequestCtx) (interface{}, error) {
	peers := h.m.Server().GetPeers()
	return peers, nil
}

// PeersPeerID returns specific info about one peer
func (h *HTTP) PeersPeerID(ctx *fasthttp.RequestCtx) (interface{}, error) {
	peeridRaw := ctx.UserValue("peerid")
	peerid, ok := peeridRaw.(string)
	if !ok {
		return nil, fmt.Errorf("expected string for peerid")
	}
	peer, ok := h.m.Server().GetPeerByPrefix(peerid)
	if !ok {
		return nil, fmt.Errorf("peer '%s' not found", peerid)
	}

	// format data
	protocols := []network.ProtocolSpec{}
	for _, p := range peer.GetProtocols() {
		protocols = append(protocols, p.Protocol.Spec)
	}
	info := map[string]interface{}{
		"client":    peer.Info.Client,
		"id":        peer.ID,
		"ip":        peer.Enode.IP.String(),
		"protocols": protocols,
	}

	return info, nil
}

// PeersAdd connects to a new peer synchronously
func (h *HTTP) PeersAdd(ctx *fasthttp.RequestCtx) (interface{}, error) {
	val := ctx.PostArgs().Peek("peer")
	if len(val) == 0 {
		return nil, fmt.Errorf("peer not found in post args")
	}

	err := h.m.Server().DialSync(string(val))
	return nil, err
}
