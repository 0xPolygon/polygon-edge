package jsonrpc

// Net is the net jsonrpc endpoint
type Net struct {
	d *Dispatcher
}

// Version returns the current network id
func (n *Net) Version() (interface{}, error) {
	//return fmt.Sprintf("%x", n.d.chainID), nil
	return argUintPtr(n.d.chainID), nil
}

// Listening returns true if client is actively listening for network connections
func (n *Net) Listening() (interface{}, error) {
	return true, nil
}

// PeerCount returns number of peers currently connected to the client
func (n *Net) PeerCount() (interface{}, error) {
	return argUintPtr(0), nil
}
