package crypto

import (
	"bytes"
	goCrypto "crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keystore"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/btcsuite/btcd/btcec"
	"golang.org/x/crypto/sha3"

	"github.com/umbracle/fastrlp"
)

var (
	big1 = big.NewInt(1)
)

// S256 is the secp256k1 elliptic curve
var S256 = btcec.S256()

var (
	secp256k1N = hex.MustDecodeHex("0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141")
	one        = []byte{0x01}
)

var (
	errInvalidSignature = errors.New("invalid signature")
)

func trimLeftZeros(b []byte) []byte {
	i := 0
	for i = range b {
		if b[i] != 0 {
			break
		}
	}

	return b[i:]
}

// ValidateSignatureValues checks if the signature values are correct
func ValidateSignatureValues(v byte, r, s *big.Int) bool {
	// TODO: ECDSA malleability
	if r == nil || s == nil {
		return false
	}

	if v > 1 {
		return false
	}

	rr := r.Bytes()
	rr = trimLeftZeros(rr)

	if bytes.Compare(rr, secp256k1N) >= 0 || bytes.Compare(rr, one) < 0 {
		return false
	}

	ss := s.Bytes()
	ss = trimLeftZeros(ss)

	if bytes.Compare(ss, secp256k1N) >= 0 || bytes.Compare(ss, one) < 0 {
		return false
	}

	return true
}

var addressPool fastrlp.ArenaPool

// CreateAddress creates an Ethereum address.
func CreateAddress(addr types.Address, nonce uint64) types.Address {
	a := addressPool.Get()
	defer addressPool.Put(a)

	v := a.NewArray()
	v.Set(a.NewBytes(addr.Bytes()))
	v.Set(a.NewUint(nonce))

	dst := v.MarshalTo(nil)
	dst = Keccak256(dst)[12:]

	return types.BytesToAddress(dst)
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

// MarshalPrivateKey serializes the private key's D value to a []byte
func MarshalPrivateKey(priv *ecdsa.PrivateKey) ([]byte, error) {
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
		return nil, fmt.Errorf("cannot unmarshal")
	}

	return &ecdsa.PublicKey{Curve: S256, X: x, Y: y}, nil
}

// MarshalPublicKey marshals a public key on the secp256k1 elliptic curve.
func MarshalPublicKey(pub *ecdsa.PublicKey) []byte {
	return elliptic.Marshal(S256, pub.X, pub.Y)
}

func Ecrecover(hash, sig []byte) ([]byte, error) {
	pub, err := RecoverPubkey(sig, hash)
	if err != nil {
		return nil, err
	}

	return MarshalPublicKey(pub), nil
}

// RecoverPubkey verifies the compact signature "signature" of "hash" for the
// secp256k1 curve.
func RecoverPubkey(signature, hash []byte) (*ecdsa.PublicKey, error) {
	size := len(signature)
	term := byte(27)

	// Make sure the signature is present
	if signature == nil || size < 1 {
		return nil, errInvalidSignature
	}

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

// SigToPub returns the public key that created the given signature.
func SigToPub(hash, sig []byte) (*ecdsa.PublicKey, error) {
	s, err := Ecrecover(hash, sig)
	if err != nil {
		return nil, err
	}

	x, y := elliptic.Unmarshal(S256, s)

	return &ecdsa.PublicKey{Curve: S256, X: x, Y: y}, nil
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
	buf := Keccak256(MarshalPublicKey(pub)[1:])[12:]

	return types.BytesToAddress(buf)
}

// GetAddressFromKey extracts an address from the private key
func GetAddressFromKey(key goCrypto.PrivateKey) (types.Address, error) {
	privateKeyConv, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return types.ZeroAddress, errors.New("unable to assert type")
	}

	publicKey := privateKeyConv.PublicKey

	return PubKeyToAddress(&publicKey), nil
}

// generateKeyAndMarshal generates a new private key and serializes it to a byte array
func generateKeyAndMarshal() ([]byte, error) {
	key, err := GenerateKey()
	if err != nil {
		return nil, err
	}

	buf, err := MarshalPrivateKey(key)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// BytesToPrivateKey reads the input byte array and constructs a private key if possible
func BytesToPrivateKey(input []byte) (*ecdsa.PrivateKey, error) {
	// The key file on disk should be encoded in Base64,
	// so it must be decoded before it can be parsed by ParsePrivateKey
	decoded, err := hex.DecodeString(string(input))
	if err != nil {
		return nil, err
	}

	// Make sure the key is properly formatted
	if len(decoded) != 32 {
		// Key must be exactly 64 chars (32B) long
		return nil, fmt.Errorf("invalid key length (%dB), should be 32B", len(decoded))
	}

	// Convert decoded bytes to a private key
	key, err := ParsePrivateKey(decoded)
	if err != nil {
		return nil, err
	}

	return key, nil
}

// GenerateOrReadPrivateKey generates a private key at the specified path,
// or reads it if a key file is present
func GenerateOrReadPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	keyBuff, err := keystore.CreateIfNotExists(path, generateKeyAndMarshal)
	if err != nil {
		return nil, err
	}

	privateKey, err := BytesToPrivateKey(keyBuff)
	if err != nil {
		return nil, fmt.Errorf("unable to execute byte array -> private key conversion, %w", err)
	}

	return privateKey, nil
}

// GenerateAndEncodePrivateKey returns a newly generated private key and the Base64 encoding of that private key
func GenerateAndEncodePrivateKey() (*ecdsa.PrivateKey, []byte, error) {
	keyBuff, err := keystore.CreatePrivateKey(generateKeyAndMarshal)
	if err != nil {
		return nil, nil, err
	}

	privateKey, err := BytesToPrivateKey(keyBuff)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to execute byte array -> private key conversion, %w", err)
	}

	return privateKey, keyBuff, nil
}

func ReadConsensusKey(manager secrets.SecretsManager) (*ecdsa.PrivateKey, error) {
	validatorKey, err := manager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return nil, err
	}

	return BytesToPrivateKey(validatorKey)
}
