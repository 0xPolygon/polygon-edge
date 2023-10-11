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

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keystore"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/btcsuite/btcd/btcec"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
	"github.com/umbracle/fastrlp"
	"golang.org/x/crypto/sha3"
)

// S256 is the secp256k1 elliptic curve
var S256 = btcec.S256()

var (
	secp256k1N, _  = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1NHalf = new(big.Int).Div(secp256k1N, big.NewInt(2))
	zero           = big.NewInt(0)
	one            = big.NewInt(1)

	ErrInvalidBLSSignature = errors.New("invalid BLS Signature")
	errZeroHash            = errors.New("can not recover public key from zero or empty message hash")
	errHashOfInvalidLength = errors.New("message hash of invalid length")
	errInvalidSignature    = errors.New("invalid signature")
)

type KeyType string

const (
	KeyECDSA KeyType = "ecdsa"
	KeyBLS   KeyType = "bls"
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
	prv, _ := btcec.PrivKeyFromBytes(S256, buf)

	return prv.ToECDSA(), nil
}

// MarshalECDSAPrivateKey serializes the private key's D value to a []byte
func MarshalECDSAPrivateKey(priv *ecdsa.PrivateKey) ([]byte, error) {
	return (*btcec.PrivateKey)(priv).Serialize(), nil
}

// GenerateECDSAKey generates a new key based on the secp256k1 elliptic curve.
func GenerateECDSAKey() (*ecdsa.PrivateKey, error) {
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
	if len(hash) != types.HashLength {
		return nil, errHashOfInvalidLength
	}

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

// SignByBLS signs the given data by BLS
func SignByBLS(prv *bls_sig.SecretKey, msg []byte) ([]byte, error) {
	signature, err := bls_sig.NewSigPop().Sign(prv, msg)
	if err != nil {
		return nil, err
	}

	return signature.MarshalBinary()
}

// VerifyBLSSignature verifies the given signature from Public Key and original message
func VerifyBLSSignature(pubkey *bls_sig.PublicKey, sig *bls_sig.Signature, message []byte) error {
	ok, err := bls_sig.NewSigPop().Verify(pubkey, message, sig)
	if err != nil {
		return err
	}

	if !ok {
		return ErrInvalidBLSSignature
	}

	return nil
}

// VerifyBLSSignatureFromBytes verifies BLS Signature from BLS PublicKey, signature, and original message in bytes
func VerifyBLSSignatureFromBytes(rawPubkey, rawSig, message []byte) error {
	pubkey, err := UnmarshalBLSPublicKey(rawPubkey)
	if err != nil {
		return err
	}

	signature, err := UnmarshalBLSSignature(rawSig)
	if err != nil {
		return err
	}

	return VerifyBLSSignature(pubkey, signature, message)
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

// GenerateBLSKey generates a new BLS key
func GenerateBLSKey() (*bls_sig.SecretKey, error) {
	blsPop := bls_sig.NewSigPop()

	_, sk, err := blsPop.Keygen()
	if err != nil {
		return nil, err
	}

	return sk, nil
}

// generateBLSKeyAndMarshal generates a new BLS secret key and serializes it to a byte array
func generateBLSKeyAndMarshal() ([]byte, error) {
	key, err := GenerateBLSKey()
	if err != nil {
		return nil, err
	}

	buf, err := key.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// BytesToBLSSecretKey reads the input byte array and constructs a private key if possible
func BytesToBLSSecretKey(input []byte) (*bls_sig.SecretKey, error) {
	// The key file on disk should be encoded in hex,
	// so it must be decoded before it can be parsed by ParsePrivateKey
	decoded, err := hex.DecodeString(string(input))
	if err != nil {
		return nil, err
	}

	sk := &bls_sig.SecretKey{}
	if err := sk.UnmarshalBinary(decoded); err != nil {
		return nil, err
	}

	return sk, nil
}

// BLSSecretKeyToPubkeyBytes returns bytes of BLS Public Key corresponding to the given secret key
func BLSSecretKeyToPubkeyBytes(key *bls_sig.SecretKey) ([]byte, error) {
	pubKey, err := key.GetPublicKey()
	if err != nil {
		return nil, err
	}

	marshalled, err := pubKey.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return marshalled, nil
}

// UnmarshalBLSPublicKey unmarshal bytes data into BLS Public Key
func UnmarshalBLSPublicKey(input []byte) (*bls_sig.PublicKey, error) {
	pk := &bls_sig.PublicKey{}
	if err := pk.UnmarshalBinary(input); err != nil {
		return nil, err
	}

	return pk, nil
}

// UnmarshalBLSSignature unmarshal bytes data into BLS Signature
func UnmarshalBLSSignature(input []byte) (*bls_sig.Signature, error) {
	sig := &bls_sig.Signature{}
	if err := sig.UnmarshalBinary(input); err != nil {
		return nil, err
	}

	return sig, nil
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

func GenerateAndEncodeBLSSecretKey() (*bls_sig.SecretKey, []byte, error) {
	keyBuff, err := keystore.CreatePrivateKey(generateBLSKeyAndMarshal)
	if err != nil {
		return nil, nil, err
	}

	secretKey, err := BytesToBLSSecretKey(keyBuff)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to execute byte array -> private key conversion, %w", err)
	}

	return secretKey, keyBuff, nil
}

func ReadConsensusKey(manager secrets.SecretsManager) (*ecdsa.PrivateKey, error) {
	validatorKey, err := manager.GetSecret(secrets.ValidatorKey)
	if err != nil {
		return nil, err
	}

	return BytesToECDSAPrivateKey(validatorKey)
}
