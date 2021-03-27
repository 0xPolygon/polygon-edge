package jsonrpc

import (
	"github.com/umbracle/go-web3/jsonrpc/transport"
)

// Client is the jsonrpc client
type Client struct {
	transport transport.Transport
	endpoints endpoints
}

type endpoints struct {
	w *Web3
	e *Eth
	n *Net
}

// NewClient creates a new client
func NewClient(addr string) (*Client, error) {
	c := &Client{}
	c.endpoints.w = &Web3{c}
	c.endpoints.e = &Eth{c}
	c.endpoints.n = &Net{c}

	t, err := transport.NewTransport(addr)
	if err != nil {
		return nil, err
	}
	c.transport = t
	return c, nil
}

// Close closes the tranport
func (c *Client) Close() error {
	return c.transport.Close()
}

// Call makes a jsonrpc call
func (c *Client) Call(method string, out interface{}, params ...interface{}) error {
	return c.transport.Call(method, out, params...)
}
