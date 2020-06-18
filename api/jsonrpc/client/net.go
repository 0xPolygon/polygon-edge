package client

import (
	"github.com/0xPolygon/minimal/types"
)

// Net is used to query the net service
type Net struct {
	client *Client
}

// Net returns a handle on the net endpoints.
func (c *Client) Net() *Net {
	return &Net{client: c}
}

// Version returns the current network id
func (c *Net) Version() (uint64, error) {
	var out string
	if err := c.client.do("net_version", &out); err != nil {
		return 0, err
	}
	return types.ParseUint64orHex(&out)
}

// Listening returns true if client is actively listening for network connections
func (c *Net) Listening() (bool, error) {
	var out bool
	err := c.client.do("net_listening", &out)
	return out, err
}

// PeerCount returns number of peers currently connected to the client
func (c *Net) PeerCount() (uint64, error) {
	var out string
	if err := c.client.do("net_peerCount", &out); err != nil {
		return 0, err
	}
	return types.ParseUint64orHex(&out)
}
