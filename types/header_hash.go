package types

import (
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/umbracle/fastrlp"
)

var HeaderHash func(h *Header) Hash

func init() {
	HeaderHash = defHeaderHash
}

var marshalArenaPool fastrlp.ArenaPool

func defHeaderHash(h *Header) (hash Hash) {
	// default header hashing
	ar := marshalArenaPool.Get()
	hasher := keccak.DefaultKeccakPool.Get()

	v := h.MarshalRLPWith(ar)
	hasher.WriteRlp(hash[:0], v)

	marshalArenaPool.Put(ar)
	keccak.DefaultKeccakPool.Put(hasher)

	return
}

// ComputeHash computes the hash of the header
func (h *Header) ComputeHash() *Header {
	h.Hash = HeaderHash(h)

	return h
}
