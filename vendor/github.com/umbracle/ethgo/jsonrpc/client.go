package jsonrpc

import (
	"github.com/umbracle/ethgo/jsonrpc/transport"
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
	d *Debug
}

type Config struct {
	headers map[string]string
}

type ConfigOption func(*Config)

func WithHeaders(headers map[string]string) ConfigOption {
	return func(c *Config) {
		for k, v := range headers {
			c.headers[k] = v
		}
	}
}

func NewClient(addr string, opts ...ConfigOption) (*Client, error) {
	config := &Config{headers: map[string]string{}}
	for _, opt := range opts {
		opt(config)
	}

	c := &Client{}
	c.endpoints.w = &Web3{c}
	c.endpoints.e = &Eth{c}
	c.endpoints.n = &Net{c}
	c.endpoints.d = &Debug{c}

	t, err := transport.NewTransport(addr, config.headers)
	if err != nil {
		return nil, err
	}
	c.transport = t
	return c, nil
}

// Close closes the transport
func (c *Client) Close() error {
	return c.transport.Close()
}

// Call makes a jsonrpc call
func (c *Client) Call(method string, out interface{}, params ...interface{}) error {
	return c.transport.Call(method, out, params...)
}

// SetMaxConnsLimit sets the maximum number of connections that can be established with a host
func (c *Client) SetMaxConnsLimit(count int) {
	c.transport.SetMaxConnsPerHost(count)
}
