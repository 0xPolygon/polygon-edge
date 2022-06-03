package jsonrpc

import (
	"fmt"

	"github.com/umbracle/ethgo/jsonrpc/transport"
)

// SubscriptionEnabled returns true if the subscription endpoints are enabled
func (c *Client) SubscriptionEnabled() bool {
	_, ok := c.transport.(transport.PubSubTransport)
	return ok
}

// Subscribe starts a new subscription
func (c *Client) Subscribe(method string, callback func(b []byte)) (func() error, error) {
	pub, ok := c.transport.(transport.PubSubTransport)
	if !ok {
		return nil, fmt.Errorf("transport does not support the subscribe method")
	}
	close, err := pub.Subscribe(method, callback)
	return close, err
}
