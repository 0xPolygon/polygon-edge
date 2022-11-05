package jsonrpc

import "strconv"

// networkStore provides methods needed for Net endpoint
type networkStore interface {
	GetPeers() int
}

// Net is the net jsonrpc endpoint
type Net struct {
	store   networkStore
	chainID uint64
}

// Version returns the current network id
func (n *Net) Version() (interface{}, error) {
	return strconv.FormatUint(n.chainID, 10), nil
}

// Listening returns true if client is actively listening for network connections
func (n *Net) Listening() (interface{}, error) {
	return true, nil
}

// PeerCount returns number of peers currently connected to the client
func (n *Net) PeerCount() (interface{}, error) {
	peers := n.store.GetPeers()

	return argUint64(peers), nil
}
