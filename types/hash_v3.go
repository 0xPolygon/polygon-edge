package types

var mixHashHash = map[Hash]func(h *Header) Hash{}

func NewMixHash(hash Hash, fn func(h *Header) Hash) {
	mixHashHash[hash] = fn
}
