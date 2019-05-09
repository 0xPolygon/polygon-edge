package http

import "github.com/valyala/fasthttp"

// PeersList returns a list of peers
func (h *HTTP) PeersList(ctx *fasthttp.RequestCtx) (interface{}, error) {
	return "Peers list", nil
}

// PeersPeerID returns specific info about one peer
func (h *HTTP) PeersPeerID(ctx *fasthttp.RequestCtx) (interface{}, error) {
	return "Peers peer id", nil
}
