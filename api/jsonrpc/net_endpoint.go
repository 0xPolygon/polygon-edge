package jsonrpc

// Net is the net jsonrpc endpoint
type Net struct {
	s *Server
}

// Version returns the current network id
func (n *Net) Version() (interface{}, error) {
	return nil, nil
}

// Listening returns true if client is actively listening for network connections
func (n *Net) Listening() (interface{}, error) {
	return nil, nil
}

// PeerCount returns number of peers currently connected to the client
func (n *Net) PeerCount() (interface{}, error) {
	return nil, nil
}
