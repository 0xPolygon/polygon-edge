package jsonrpc

import (
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
)

// Web3 is the web3 jsonrpc endpoint
type Web3 struct {
	d *Dispatcher
}

// ClientVersion returns the current client version
func (w *Web3) ClientVersion() (interface{}, error) {
	return nil, nil
}

// Sha3 returns Keccak-256 (not the standardized SHA3-256) of the given data
func (w *Web3) Sha3(val string) (interface{}, error) {
	v, err := hex.DecodeHex(val)
	if err != nil {
		return nil, err
	}
	dst := keccak.Keccak256(nil, v)
	return hex.EncodeToHex(dst), nil
}
