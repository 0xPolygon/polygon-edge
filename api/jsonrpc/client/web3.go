package client

// Web3 is used to query the web3 service
type Web3 struct {
	client *Client
}

// Web3 returns a handle on the web3 endpoints.
func (c *Client) Web3() *Web3 {
	return &Web3{client: c}
}

// ClientVersion returns the current client version
func (c *Web3) ClientVersion() (string, error) {
	var out string
	err := c.client.do("web3_clientVersion", &out)
	return out, err
}

// Sha3 returns Keccak-256 (not the standardized SHA3-256) of the given data
func (c *Web3) Sha3(val string) (string, error) {
	var out string
	err := c.client.do("web3_sha3", &out, val)
	return out, err
}
