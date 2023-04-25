package keccak

import (
	"sync"

	"github.com/umbracle/fastrlp"
)

// DefaultKeccakPool is a default pool
var DefaultKeccakPool Pool

// Pool is a pool of keccaks
type Pool struct {
	pool sync.Pool
}

// Get returns the keccak
func (p *Pool) Get() *Keccak {
	v := p.pool.Get()
	if v == nil {
		return NewKeccak256()
	}

	keccakVal, ok := v.(*Keccak)
	if !ok {
		return nil
	}

	return keccakVal
}

// Put releases the keccak
func (p *Pool) Put(k *Keccak) {
	k.Reset()
	p.pool.Put(k)
}

// Keccak256 hashes a src with keccak-256
func Keccak256(dst, src []byte) []byte {
	h := DefaultKeccakPool.Get()
	h.Write(src) //nolint:errcheck
	dst = h.Sum(dst)
	DefaultKeccakPool.Put(h)

	return dst
}

// Keccak256Rlp hashes a fastrlp.Value with keccak-256
func Keccak256Rlp(dst []byte, src *fastrlp.Value) []byte {
	h := DefaultKeccakPool.Get()
	dst = h.WriteRlp(dst, src)
	DefaultKeccakPool.Put(h)

	return dst
}

// PrefixedKeccak256Rlp hashes a fastrlp.Value using keccak-256 with the given prefix
func PrefixedKeccak256Rlp(prefix, dst []byte, src *fastrlp.Value) []byte {
	h := DefaultKeccakPool.Get()
	_, _ = h.Write(prefix)
	dst = h.WriteRlp(dst, src)
	DefaultKeccakPool.Put(h)

	return dst
}
