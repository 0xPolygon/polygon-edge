package bls

import (
	"crypto/sha256"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	bn256 "github.com/umbracle/go-eth-bn256"
)

var (
	zeroBigInt = *big.NewInt(0)
	oneBigInt  = *big.NewInt(1)
	g2         = *new(bn256.G2).ScalarBaseMult(&oneBigInt)  // Generator point of G2 group
	zeroG1     = *new(bn256.G1).ScalarBaseMult(&zeroBigInt) // Zero point in G2 group
	zeroG2     = *new(bn256.G2).ScalarBaseMult(&zeroBigInt) // Zero point in G2 group

	// this is a copy of point p is a prime over which we form a basic field: 36u⁴+36u³+24u²+6u+1.
	// TODO delete this point and use bn256.p
	bn256P, _ = new(big.Int).SetString("21888242871839275222246405745257275088696311157297823662689037894645226208583", 10)
)

func sum(ints ...*big.Int) *big.Int {
	acc := big.NewInt(0)
	for _, num := range ints {
		acc.Add(acc, num)
	}

	return acc
}

func product(ints ...*big.Int) *big.Int {
	acc := big.NewInt(1)
	for _, num := range ints {
		acc.Mul(acc, num)
	}

	return acc
}

func mod(i, m *big.Int) *big.Int {
	return new(big.Int).Mod(i, m)
}

// modSqrt returns square root of x mod p if such a square root exists. The
// modulus p must be an odd prime. If x is not a square mod p, function returns
// nil.
func modSqrt(x, p *big.Int) *big.Int {
	return new(big.Int).ModSqrt(x, p)
}

// yFromX calculates and returns only one of the two possible Ys, by
// solving the curve equation for X, the two Ys can be distinguished by
// their parity.
func yFromX(x *big.Int) *big.Int {
	return modSqrt(sum(product(x, x, x), big.NewInt(3)), bn256P)
}

// g1FromInts returns G1 point based on the provided x and y.
func g1FromInts(x *big.Int, y *big.Int) (*bn256.G1, error) {
	const size = 32

	if len(x.Bytes()) > size || len(y.Bytes()) > size {
		return nil, errors.New("points on G1 are limited to 256-bit coordinates")
	}

	paddedX := common.PadLeftOrTrim(x.Bytes(), size)
	paddedY := common.PadLeftOrTrim(y.Bytes(), size)

	m := append(paddedX, paddedY...)

	g1 := new(bn256.G1)

	_, err := g1.Unmarshal(m)
	if err != nil {
		return nil, err
	}

	return g1, nil
}

// g1HashToPoint hashes the provided byte slice, maps it into a G1
// and returns it as a G1 point.
func g1HashToPoint(m []byte) (*bn256.G1, error) {
	one := big.NewInt(1)
	h := sha256.Sum256(m)
	x := mod(new(big.Int).SetBytes(h[:]), bn256P)

	for {
		y := yFromX(x)
		if y != nil {
			return g1FromInts(x, y)
		}

		x.Add(x, one)
	}
}

// colects public keys from the BlsKeys
func collectPublicKeys(keys []*PrivateKey) []*PublicKey {
	pubKeys := make([]*PublicKey, len(keys))

	for i, key := range keys {
		pubKeys[i] = key.PublicKey()
	}

	return pubKeys
}
