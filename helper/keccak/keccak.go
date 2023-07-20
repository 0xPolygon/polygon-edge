package keccak

import (
	"hash"

	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"
)

type hashImpl interface {
	hash.Hash
	Read(b []byte) (int, error)
}

// Keccak is the sha256 keccak hash
type Keccak struct {
	buf  []byte // buffer to store intermediate rlp marshal values
	tmp  []byte
	hash hashImpl
}

// WriteRlp writes an RLP value
func (k *Keccak) WriteRlp(dst []byte, v *fastrlp.Value) []byte {
	k.buf = v.MarshalTo(k.buf[:0])
	k.Write(k.buf) //nolint:errcheck

	return k.Sum(dst)
}

// WriteFn writes using value provided by custom marshalFn func
func (k *Keccak) WriteFn(dst []byte, marshalFn func([]byte) []byte) []byte {
	k.buf = marshalFn(k.buf[:0])
	k.Write(k.buf) //nolint:errcheck

	return k.Sum(dst)
}

// Write implements the hash interface
func (k *Keccak) Write(b []byte) (int, error) {
	return k.hash.Write(b)
}

// Reset implements the hash interface
func (k *Keccak) Reset() {
	k.buf = k.buf[:0]
	k.hash.Reset()
}

// Read hashes the content and returns the intermediate buffer.
func (k *Keccak) Read() []byte {
	k.hash.Read(k.tmp) //nolint:errcheck

	return k.tmp
}

// Sum implements the hash interface
func (k *Keccak) Sum(dst []byte) []byte {
	k.hash.Read(k.tmp) //nolint:errcheck
	dst = append(dst, k.tmp[:]...)

	return dst
}

func newKeccak(hash hashImpl) *Keccak {
	return &Keccak{
		hash: hash,
		tmp:  make([]byte, hash.Size()),
	}
}

// NewKeccak256 returns a new keccak 256
func NewKeccak256() *Keccak {
	impl, ok := sha3.NewLegacyKeccak256().(hashImpl)
	if !ok {
		return nil
	}

	return newKeccak(impl)
}
