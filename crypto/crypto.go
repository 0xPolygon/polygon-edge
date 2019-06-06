package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec"
	"github.com/umbracle/ecies"
	"github.com/umbracle/minimal/types"
	"golang.org/x/crypto/sha3"

	"github.com/umbracle/minimal/rlp"
)

var (
	big1 = big.NewInt(1)
)

func init() {
	ecies.AddParamsForCurve(S256, ecies.ECIES_AES128_SHA256)
}

// S256 is the secp256k1 elliptic curve
var S256 = btcec.S256()

var (
	secp256k1N, _ = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
)

// ValidateSignatureValues checks if the signature values are correct
func ValidateSignatureValues(v byte, r, s *big.Int, homestead bool) bool {
	// TODO: ECDSA malleability
	if v > 1 {
		return false
	}
	if r.Cmp(secp256k1N) >= 0 || r.Cmp(big1) < 0 {
		return false
	}
	if s.Cmp(secp256k1N) >= 0 || s.Cmp(big1) < 0 {
		return false
	}
	return true
}

// CreateAddress creates an Ethereum address.
func CreateAddress(addr types.Address, nonce uint64) types.Address {
	buf, _ := rlp.EncodeToBytes([]interface{}{
		addr,
		nonce,
	})
	return types.BytesToAddress(Keccak256(buf)[12:])
}

var create2Prefix = []byte{0xff}

// CreateAddress2 creates an Ethereum address following the CREATE2 Opcode.
func CreateAddress2(addr types.Address, salt [32]byte, inithash []byte) types.Address {
	return types.BytesToAddress(Keccak256(create2Prefix, addr.Bytes(), salt[:], Keccak256(inithash))[12:])
}

func ParsePrivateKey(buf []byte) (*ecdsa.PrivateKey, error) {
	prv, _ := btcec.PrivKeyFromBytes(S256, buf)
	return prv.ToECDSA(), nil
}

func MarshallPrivateKey(priv *ecdsa.PrivateKey) ([]byte, error) {
	return (*btcec.PrivateKey)(priv).Serialize(), nil
}

// GenerateKey generates a new key based on the secp256k1 elliptic curve.
func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(S256, rand.Reader)
}

// ParsePublicKey parses bytes into a public key on the secp256k1 elliptic curve.
func ParsePublicKey(buf []byte) (*ecdsa.PublicKey, error) {
	x, y := elliptic.Unmarshal(S256, buf)
	if x == nil || y == nil {
		return nil, fmt.Errorf("cannot unmarshall")
	}
	return &ecdsa.PublicKey{Curve: S256, X: x, Y: y}, nil
}

// MarshallPublicKey marshalls a public key on the secp256k1 elliptic curve.
func MarshallPublicKey(pub *ecdsa.PublicKey) []byte {
	return elliptic.Marshal(S256, pub.X, pub.Y)
}

func Ecrecover(hash, sig []byte) ([]byte, error) {
	pub, err := RecoverPubkey(sig, hash)
	if err != nil {
		return nil, err
	}
	return MarshallPublicKey(pub), nil
}

// RecoverPubkey verifies the compact signature "signature" of "hash" for the
// secp256k1 curve.
func RecoverPubkey(signature, hash []byte) (*ecdsa.PublicKey, error) {
	size := len(signature)
	term := byte(27)
	if signature[size-1] == 1 {
		term = 28
	}

	sig := append([]byte{term}, signature[:size-1]...)
	pub, _, err := btcec.RecoverCompact(S256, sig, hash)
	if err != nil {
		return nil, err
	}
	return pub.ToECDSA(), nil
}

// Sign produces a compact signature of the data in hash with the given
// private key on the secp256k1 curve.
func Sign(priv *ecdsa.PrivateKey, hash []byte) ([]byte, error) {
	sig, err := btcec.SignCompact(S256, (*btcec.PrivateKey)(priv), hash, false)
	if err != nil {
		return nil, err
	}
	term := byte(0)
	if sig[0] == 28 {
		term = 1
	}
	return append(sig, term)[1:], nil
}

// Keccak256 calculates the Keccak256
func Keccak256(v ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, i := range v {
		h.Write(i)
	}
	return h.Sum(nil)
}

// PubKeyToAddress returns the Ethereum address of a public key
func PubKeyToAddress(pub *ecdsa.PublicKey) types.Address {
	buf := Keccak256(MarshallPublicKey(pub)[1:])[12:]
	return types.BytesToAddress(buf)
}
