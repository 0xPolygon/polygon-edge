package native

import (
	"hash"

	"golang.org/x/crypto/sha3"
)

// OversizeDstSalt is the salt used to hash a dst over MaxDstLen
var OversizeDstSalt = []byte("H2C-OVERSIZE-DST-")

// MaxDstLen the max size for dst in hash to curve
const MaxDstLen = 255

func getDomainXmd(h hash.Hash, domain []byte) []byte {
	var out []byte
	if len(domain) > MaxDstLen {
		h.Reset()
		_, _ = h.Write(OversizeDstSalt)
		_, _ = h.Write(domain)
		out = h.Sum(nil)
	} else {
		out = domain
	}
	return out
}

func getDomainXof(h sha3.ShakeHash, domain []byte) []byte {
	var out []byte
	if len(domain) > MaxDstLen {
		h.Reset()
		_, _ = h.Write(OversizeDstSalt)
		_, _ = h.Write(domain)
		var tv [64]byte
		_, _ = h.Read(tv[:])
		out = tv[:]
	} else {
		out = domain
	}
	return out
}

// ExpandMsgXmd expands the msg with the domain to output a byte array
// with outLen in size using a fixed size hash.
// See https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-13#section-5.4.1
func ExpandMsgXmd(h *EllipticPointHasher, msg, domain []byte, outLen int) []byte {
	domain = getDomainXmd(h.xmd, domain)
	domainLen := byte(len(domain))
	h.xmd.Reset()
	// DST_prime = DST || I2OSP(len(DST), 1)
	// b_0 = H(Z_pad || msg || l_i_b_str || I2OSP(0, 1) || DST_prime)
	_, _ = h.xmd.Write(make([]byte, h.xmd.BlockSize()))
	_, _ = h.xmd.Write(msg)
	_, _ = h.xmd.Write([]byte{uint8(outLen >> 8), uint8(outLen)})
	_, _ = h.xmd.Write([]byte{0})
	_, _ = h.xmd.Write(domain)
	_, _ = h.xmd.Write([]byte{domainLen})
	b0 := h.xmd.Sum(nil)

	// b_1 = H(b_0 || I2OSP(1, 1) || DST_prime)
	h.xmd.Reset()
	_, _ = h.xmd.Write(b0)
	_, _ = h.xmd.Write([]byte{1})
	_, _ = h.xmd.Write(domain)
	_, _ = h.xmd.Write([]byte{domainLen})
	b1 := h.xmd.Sum(nil)

	// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
	ell := (outLen + h.xmd.Size() - 1) / h.xmd.Size()
	bi := b1
	out := make([]byte, outLen)
	for i := 1; i < ell; i++ {
		h.xmd.Reset()
		// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
		tmp := make([]byte, h.xmd.Size())
		for j := 0; j < h.xmd.Size(); j++ {
			tmp[j] = b0[j] ^ bi[j]
		}
		_, _ = h.xmd.Write(tmp)
		_, _ = h.xmd.Write([]byte{1 + uint8(i)})
		_, _ = h.xmd.Write(domain)
		_, _ = h.xmd.Write([]byte{domainLen})

		// b_1 || ... || b_(ell - 1)
		copy(out[(i-1)*h.xmd.Size():i*h.xmd.Size()], bi[:])
		bi = h.xmd.Sum(nil)
	}
	// b_ell
	copy(out[(ell-1)*h.xmd.Size():], bi[:])
	return out[:outLen]
}

// ExpandMsgXof expands the msg with the domain to output a byte array
// with outLen in size using a xof hash
// See https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-13#section-5.4.2
func ExpandMsgXof(h *EllipticPointHasher, msg, domain []byte, outLen int) []byte {
	domain = getDomainXof(h.xof, domain)
	domainLen := byte(len(domain))
	h.xof.Reset()
	_, _ = h.xof.Write(msg)
	_, _ = h.xof.Write([]byte{uint8(outLen >> 8), uint8(outLen)})
	_, _ = h.xof.Write(domain)
	_, _ = h.xof.Write([]byte{domainLen})
	out := make([]byte, outLen)
	_, _ = h.xof.Read(out)
	return out
}
