package jsonrpc

// Web3 is the web3 namespace
type Web3 struct {
	c *Client
}

// Web3 returns the reference to the web3 namespace
func (c *Client) Web3() *Web3 {
	return c.endpoints.w
}

// ClientVersion returns the current client version
func (w *Web3) ClientVersion() (string, error) {
	var out string
	err := w.c.Call("web3_clientVersion", &out)
	return out, err
}

// Sha3 returns Keccak-256 (not the standardized SHA3-256) of the given data
func (w *Web3) Sha3(val []byte) ([]byte, error) {
	var out string
	if err := w.c.Call("web3_sha3", &out, encodeToHex(val)); err != nil {
		return nil, err
	}
	return parseHexBytes(out)
}
