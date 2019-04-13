package jsonrpc

import (
	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Web3 is the web3 jsonrpc endpoint
type Web3 struct {
	s *Server
}

// ClientVersion returns the current client version
func (w *Web3) ClientVersion() (interface{}, error) {
	return nil, nil
}

// Sha3 returns Keccak-256 (not the standardized SHA3-256) of the given data
func (w *Web3) Sha3(val string) (interface{}, error) {
	v, err := hexutil.Decode(val)
	if err != nil {
		return nil, err
	}

	h := sha3.NewLegacyKeccak256()
	h.Write(v)
	return hexutil.Encode(h.Sum(nil)), nil
}
