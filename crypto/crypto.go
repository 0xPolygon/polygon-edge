package crypto

import (
	goCrypto "crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"hash"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	btc_ecdsa "github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keystore"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	secp256k1N, _  = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1NHalf = new(big.Int).Div(secp256k1N, big.NewInt(2))
	zero           = big.NewInt(0)
	one            = big.NewInt(1)

	ErrInvalidBLSSignature = errors.New("invalid BLS Signature")
	errHashOfInvalidLength = errors.New("message hash of invalid length")
	errInvalidSignature    = errors.New("invalid signature")
)

type KeyType string

const (
	KeyECDSA KeyType = "ecdsa"
	KeyBLS   KeyType = "bls"
)

const (
	// ECDSASignatureLength indicates the byte length required to carry a signature with recovery id.
	// (64 bytes ECDSA signature + 1 byte recovery id)
	ECDSASignatureLength = 64 + 1

	// recoveryID is ECDSA signature recovery id
	recoveryID = byte(27)

	// recoveryIDOffset points to the byte offset within the signature that contains the recovery id.
	recoveryIDOffset = 64
)

// KeccakState wraps sha3.state. In addition to the usual hash methods, it also supports
// Read to get a variable amount of data from the hash state. Read is faster than Sum
// because it doesn't copy the internal state, but also modifies the internal state.
type KeccakState interface {
	hash.Hash
	Read([]byte) (int, error)
}

// ValidateSignatureValues checks if the signature values are correct
func ValidateSignatureValues(v, r, s *big.Int, isHomestead bool) bool {
	// r & s must not be nil
	if r == nil || s == nil {
		return false
	}

	// r & s must be positive integer
	if r.Cmp(one) < 0 || s.Cmp(one) < 0 {
		return false
	}

	// v must be 0 or 1
	if v.Cmp(zero) == -1 || v.Cmp(one) == 1 {
		return false
	}

	// From Homestead, s must be less or equal than secp256k1n/2
	if isHomestead {
		return r.Cmp(secp256k1N) < 0 && s.Cmp(secp256k1NHalf) <= 0
	}

	// In Frontier, r and s must be less than secp256k1n
	return r.Cmp(secp256k1N) < 0 && s.Cmp(secp256k1N) < 0
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

func ParseECDSAPrivateKey(buf []byte) (*ecdsa.PrivateKey, error) {
	prv, _ := btcec.PrivKeyFromBytes(buf)

	return prv.ToECDSA(), nil
}

// MarshalECDSAPrivateKey serializes the private key's D value to a []byte
func MarshalECDSAPrivateKey(priv *ecdsa.PrivateKey) ([]byte, error) {
	btcPriv, err := convertToBtcPrivKey(priv)
	if err != nil {
		return nil, err
	}

	defer btcPriv.Zero()

	return btcPriv.Serialize(), nil
}

// GenerateECDSAKey generates a new key based on the secp256k1 elliptic curve.
func GenerateECDSAKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(btcec.S256(), rand.Reader)
}

// ParsePublicKey parses bytes into a public key on the secp256k1 elliptic curve.
func ParsePublicKey(buf []byte) (*ecdsa.PublicKey, error) {
	x, y := elliptic.Unmarshal(btcec.S256(), buf)
	if x == nil || y == nil {
		return nil, fmt.Errorf("cannot unmarshal")
	}

	return &ecdsa.PublicKey{Curve: btcec.S256(), X: x, Y: y}, nil
}

// MarshalPublicKey marshals a public key on the secp256k1 elliptic curve.
func MarshalPublicKey(pub *ecdsa.PublicKey) []byte {
	return elliptic.Marshal(btcec.S256(), pub.X, pub.Y)
}

func Ecrecover(hash, sig []byte) ([]byte, error) {
	pub, err := RecoverPubKey(sig, hash)
	if err != nil {
		return nil, err
	}

	return MarshalPublicKey(pub), nil
}

// RecoverPubKey verifies the compact signature "signature" of "hash" for the secp256k1 curve.
func RecoverPubKey(signature, hash []byte) (*ecdsa.PublicKey, error) {
	if len(hash) != types.HashLength {
		return nil, errHashOfInvalidLength
	}

	signatureSize := len(signature)
	if signatureSize != ECDSASignatureLength {
		return nil, errInvalidSignature
	}

	// Convert to btcec input format with 'recovery id' v at the beginning.
	btcsig := make([]byte, signatureSize)
	btcsig[0] = signature[signatureSize-1] + recoveryID
	copy(btcsig[1:], signature)

	pub, _, err := btc_ecdsa.RecoverCompact(btcsig, hash)
	if err != nil {
		return nil, err
	}

	return pub.ToECDSA(), nil
}

// Sign produces an ECDSA signature of the data in hash with the given
// private key on the secp256k1 curve.

// The produced signature is in the [R || S || V] format where V is 0 or 1.
func Sign(priv *ecdsa.PrivateKey, hash []byte) ([]byte, error) {
	if len(hash) != types.HashLength {
		return nil, fmt.Errorf("hash is required to be exactly %d bytes (%d)", types.HashLength, len(hash))
	}

	if priv.Curve != btcec.S256() {
		return nil, errors.New("private key curve is not secp256k1")
	}

	// convert from ecdsa.PrivateKey to btcec.PrivateKey
	btcPrivKey, err := convertToBtcPrivKey(priv)
	if err != nil {
		return nil, err
	}

	defer btcPrivKey.Zero()

	sig, err := btc_ecdsa.SignCompact(btcPrivKey, hash, false)
	if err != nil {
		return nil, err
	}

	// Convert to Ethereum signature format with 'recovery id' v at the end.
	v := sig[0] - recoveryID
	copy(sig, sig[1:])
	sig[recoveryIDOffset] = v

	return sig, nil
}

// Keccak256 calculates the Keccak256
func Keccak256(v ...[]byte) []byte {
	h := sha3.NewLegacyKeccak256()
	for _, i := range v {
		h.Write(i)
	}

	return h.Sum(nil)
}

// Keccak256Hash calculates and returns the Keccak256 hash of the input data,
// converting it to an internal Hash data structure.
func Keccak256Hash(v ...[]byte) (hash types.Hash) {
	h := NewKeccakState()
	for _, b := range v {
		h.Write(b)
	}

	h.Read(hash[:]) //nolint:errcheck

	return hash
}

// NewKeccakState creates a new KeccakState
func NewKeccakState() KeccakState {
	return sha3.NewLegacyKeccak256().(KeccakState) //nolint:forcetypeassert
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

// generateECDSAKeyAndMarshal generates a new ECDSA private key and serializes it to a byte array
func generateECDSAKeyAndMarshal() ([]byte, error) {
	key, err := GenerateECDSAKey()
	if err != nil {
		return nil, err
	}

	buf, err := MarshalECDSAPrivateKey(key)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// BytesToECDSAPrivateKey reads the input byte array and constructs a private key if possible
func BytesToECDSAPrivateKey(input []byte) (*ecdsa.PrivateKey, error) {
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
	key, err := ParseECDSAPrivateKey(decoded)
	if err != nil {
		return nil, err
	}

	return key, nil
}

// GenerateOrReadPrivateKey generates a private key at the specified path,
// or reads it if a key file is present
func GenerateOrReadPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	keyBuff, err := keystore.CreateIfNotExists(path, generateECDSAKeyAndMarshal)
	if err != nil {
		return nil, err
	}

	privateKey, err := BytesToECDSAPrivateKey(keyBuff)
	if err != nil {
		return nil, fmt.Errorf("unable to execute byte array -> private key conversion, %w", err)
	}

	return privateKey, nil
}

// GenerateAndEncodeECDSAPrivateKey returns a newly generated private key and the Base64 encoding of that private key
func GenerateAndEncodeECDSAPrivateKey() (*ecdsa.PrivateKey, []byte, error) {
	keyBuff, err := keystore.CreatePrivateKey(generateECDSAKeyAndMarshal)
	if err != nil {
		return nil, nil, err
	}

	privateKey, err := BytesToECDSAPrivateKey(keyBuff)
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

	return BytesToECDSAPrivateKey(validatorKey)
}

// convertToBtcPrivKey converts provided ECDSA private key to btc private key format
// used by btcec library
func convertToBtcPrivKey(priv *ecdsa.PrivateKey) (*btcec.PrivateKey, error) {
	var btcPriv btcec.PrivateKey

	overflow := btcPriv.Key.SetByteSlice(priv.D.Bytes())
	if overflow || btcPriv.Key.IsZero() {
		return nil, errors.New("invalid private key")
	}

	return &btcPriv, nil
}
