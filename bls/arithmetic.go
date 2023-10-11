package bls

import (
	"crypto/sha256"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	bn256 "github.com/umbracle/go-eth-bn256"
)

var (
	z0, _ = new(big.Int).SetString("0000000000000000b3c4d79d41a91759a9e4c7e359b6b89eaec68e62effffffd", 16)

	z1, _ = new(big.Int).SetString("000000000000000059e26bcea0d48bacd4f263f1acdb5c4f5763473177fffffe", 16)

	// pPrime is a prime over which we form a basic field: 36u⁴+36u³+24u²+6u+1.
	pPrime, _ = new(big.Int).SetString("21888242871839275222246405745257275088696311157297823662689037894645226208583", 10)

	modulus, _ = new(big.Int).SetString("30644e72e131a029b85045b68181585d97816a916871ca8d3c208c16d87cfd47", 16)

	zero = new(big.Int).SetInt64(0)
)

// hashToPoint maps hash message and domain to G1 point
// Based on https://github.com/thehubbleproject/ and kilic library
// https://datatracker.ietf.org/doc/html/draft-irtf-cfrg-hash-to-curve-07
func hashToPoint(msg, domain []byte) (*bn256.G1, error) {
	res, err := hashToFpXMDSHA256(msg, domain, 2)
	if err != nil {
		return nil, err
	}

	//a = a.Mod(a, modulus)
	//b = b.Mod(b, modulus)
	a, b := res[0], res[1]

	g1, err := mapToG1Point(a)
	if err != nil {
		return nil, err
	}

	g2, err := mapToG1Point(b)
	if err != nil {
		return nil, err
	}

	g1.Add(g1, g2)

	return g1, nil
}

func mapToG1Point(b *big.Int) (*bn256.G1, error) {
	xx, yy, err := mapToPoint(b)
	if err != nil {
		return nil, err
	}

	pointBytes := [64]byte{}
	copy(pointBytes[:], common.PadLeftOrTrim(xx.Bytes(), 32))
	copy(pointBytes[32:], common.PadLeftOrTrim(yy.Bytes(), 32))

	g1 := new(bn256.G1)

	if _, err := g1.Unmarshal(pointBytes[:]); err != nil {
		return nil, err
	}

	return g1, nil
}

func hashToFpXMDSHA256(msg []byte, domain []byte, count int) ([]*big.Int, error) {
	randBytes, err := expandMsgSHA256XMD(msg, domain, count*48)
	if err != nil {
		return nil, err
	}

	els := make([]*big.Int, count)

	for i := 0; i < count; i++ {
		num := new(big.Int).SetBytes(randBytes[i*48 : (i+1)*48])

		// fast path
		c := num.Cmp(modulus)
		if c == 0 {
			// nothing
		} else if c != 1 && num.Cmp(zero) != -1 {
			// 0 < v < q
		} else {
			num = num.Mod(num, modulus)
		}

		// copy input + modular reduction
		els[i] = num
	}

	return els, nil
}

func expandMsgSHA256XMD(msg []byte, domain []byte, outLen int) ([]byte, error) {
	if len(domain) > 255 {
		return nil, errors.New("invalid domain length")
	}

	h := sha256.New()

	domainLen := uint8(len(domain))
	// DST_prime = DST || I2OSP(len(DST), 1)
	// b_0 = H(Z_pad || msg || l_i_b_str || I2OSP(0, 1) || DST_prime)
	_, _ = h.Write(make([]byte, h.BlockSize()))
	_, _ = h.Write(msg)
	_, _ = h.Write([]byte{uint8(outLen >> 8), uint8(outLen)})
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(domain)
	_, _ = h.Write([]byte{domainLen})
	b0 := h.Sum(nil)

	// b_1 = H(b_0 || I2OSP(1, 1) || DST_prime)
	h.Reset()
	_, _ = h.Write(b0)
	_, _ = h.Write([]byte{1})
	_, _ = h.Write(domain)
	_, _ = h.Write([]byte{domainLen})
	b1 := h.Sum(nil)

	// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
	ell := (outLen + h.Size() - 1) / h.Size()
	bi := b1
	out := make([]byte, outLen)

	for i := 1; i < ell; i++ {
		h.Reset()
		// b_i = H(strxor(b_0, b_(i - 1)) || I2OSP(i, 1) || DST_prime)
		tmp := make([]byte, h.Size())

		for j := 0; j < h.Size(); j++ {
			tmp[j] = b0[j] ^ bi[j]
		}

		_, _ = h.Write(tmp)
		_, _ = h.Write([]byte{1 + uint8(i)})
		_, _ = h.Write(domain)
		_, _ = h.Write([]byte{domainLen})

		// b_1 || ... || b_(ell - 1)
		copy(out[(i-1)*h.Size():i*h.Size()], bi[:])
		bi = h.Sum(nil)
	}

	// b_ell
	copy(out[(ell-1)*h.Size():], bi[:])

	return out[:outLen], nil
}

func mulmod(x, y, N *big.Int) *big.Int {
	xx := new(big.Int).Mul(x, y)

	return xx.Mod(xx, N)
}

func addmod(x, y, N *big.Int) *big.Int {
	xx := new(big.Int).Add(x, y)

	return xx.Mod(xx, N)
}

func inversemod(x, N *big.Int) *big.Int {
	return new(big.Int).ModInverse(x, N)
}

/**
 * @notice returns square root of a uint256 value
 * @param xx the value to take the square root of
 * @return x the uint256 value of the root
 * @return hasRoot a bool indicating if there is a square root
 */
func sqrt(xx *big.Int) (x *big.Int, hasRoot bool) {
	x = new(big.Int).ModSqrt(xx, pPrime)
	hasRoot = x != nil && mulmod(x, x, pPrime).Cmp(xx) == 0

	return
}

//	// sqrt(-3)
//
// // prettier-ignore
// uint256 private constant Z0 = 0x0000000000000000b3c4d79d41a91759a9e4c7e359b6b89eaec68e62effffffd;
// // (sqrt(-3) - 1)  / 2
// // prettier-ignore
// uint256 private constant Z1 = 0x000000000000000059e26bcea0d48bacd4f263f1acdb5c4f5763473177fffffe;
func mapToPoint(x *big.Int) (*big.Int, *big.Int, error) {
	_, decision := sqrt(x)

	// N := P
	//         uint256 a0 = mulmod(x, x, N);
	a0 := mulmod(x, x, pPrime)
	//        a0 = addmod(a0, 4, N);
	a0 = addmod(a0, big.NewInt(4), pPrime)
	//        uint256 a1 = mulmod(x, Z0, N);
	a1 := mulmod(x, z0, pPrime)
	//        uint256 a2 = mulmod(a1, a0, N);
	a2 := mulmod(a1, a0, pPrime)
	//        a2 = inverse(a2);
	a2 = inversemod(a2, pPrime)
	//        a1 = mulmod(a1, a1, N);
	a1 = mulmod(a1, a1, pPrime)
	//        a1 = mulmod(a1, a2, N);
	a1 = mulmod(a1, a2, pPrime)

	//         // x1
	//        a1 = mulmod(x, a1, N);
	a1 = mulmod(x, a1, pPrime)
	//        x = addmod(Z1, N - a1, N);
	x = addmod(z1, new(big.Int).Sub(pPrime, a1), pPrime)
	//        // check curve
	//        a1 = mulmod(x, x, N);
	a1 = mulmod(x, x, pPrime)
	//        a1 = mulmod(a1, x, N);
	a1 = mulmod(a1, x, pPrime)
	//        a1 = addmod(a1, 3, N);
	a1 = addmod(a1, big.NewInt(3), pPrime)
	//        bool found;
	//        (a1, found) = sqrt(a1);
	var found bool
	//        if (found) {
	//            if (!decision) {
	//                a1 = N - a1;
	//            }
	//            return [x, a1];
	//        }
	if a1, found = sqrt(a1); found {
		if !decision {
			a1 = new(big.Int).Sub(pPrime, a1)
		}

		return x, a1, nil
	}

	//         // x2
	//        x = N - addmod(x, 1, N);
	x = new(big.Int).Sub(pPrime, addmod(x, big.NewInt(1), pPrime))
	//        // check curve
	//        a1 = mulmod(x, x, N);
	a1 = mulmod(x, x, pPrime)
	//        a1 = mulmod(a1, x, N);
	a1 = mulmod(a1, x, pPrime)
	//        a1 = addmod(a1, 3, N);
	a1 = addmod(a1, big.NewInt(3), pPrime)
	//        (a1, found) = sqrt(a1);
	//        if (found) {
	//            if (!decision) {
	//                a1 = N - a1;
	//            }
	//            return [x, a1];
	//        }
	if a1, found = sqrt(a1); found {
		if !decision {
			a1 = new(big.Int).Sub(pPrime, a1)
		}

		return x, a1, nil
	}

	//         // x3
	//        x = mulmod(a0, a0, N);
	x = mulmod(a0, a0, pPrime)
	//        x = mulmod(x, x, N);
	x = mulmod(x, x, pPrime)
	//        x = mulmod(x, a2, N);
	x = mulmod(x, a2, pPrime)
	//        x = mulmod(x, a2, N);
	x = mulmod(x, a2, pPrime)
	//        x = addmod(x, 1, N);
	x = addmod(x, big.NewInt(1), pPrime)

	//        // must be on curve
	//        a1 = mulmod(x, x, N);
	a1 = mulmod(x, x, pPrime)

	//        a1 = mulmod(a1, x, N);
	a1 = mulmod(a1, x, pPrime)

	//        a1 = addmod(a1, 3, N);
	a1 = addmod(a1, big.NewInt(3), pPrime)

	//        (a1, found) = sqrt(a1);
	if a1, found = sqrt(a1); !found {
		return nil, nil, errors.New("bad ft mapping implementation")
	}

	//        if (!decision) {
	//            a1 = N - a1;
	//        }
	//        return [x, a1];
	if !decision {
		a1 = new(big.Int).Sub(pPrime, a1)
	}

	return x, a1, nil
}
