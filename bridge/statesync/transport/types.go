package transport

import "github.com/0xPolygon/polygon-edge/types"

type SignedMessage struct {
	Hash      types.Hash
	Signature []byte
}
