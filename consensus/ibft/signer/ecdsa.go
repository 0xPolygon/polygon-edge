package signer

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/umbracle/fastrlp"
)

// ECDSAKeyManager is a module that holds ECDSA key
// and implements methods of signing by this key
type ECDSAKeyManager struct {
	key     *ecdsa.PrivateKey
	address types.Address
}

// NewECDSAKeyManager initializes ECDSAKeyManager by the ECDSA key loaded from SecretsManager
func NewECDSAKeyManager(manager secrets.SecretsManager) (KeyManager, error) {
	key, err := getOrCreateECDSAKey(manager)
	if err != nil {
		return nil, err
	}

	return NewECDSAKeyManagerFromKey(key), nil
}

// NewECDSAKeyManagerFromKey initializes ECDSAKeyManager from the given ECDSA key
func NewECDSAKeyManagerFromKey(key *ecdsa.PrivateKey) KeyManager {
	return &ECDSAKeyManager{
		key:     key,
		address: crypto.PubKeyToAddress(&key.PublicKey),
	}
}

// Type returns the validator type KeyManager supports
func (s *ECDSAKeyManager) Type() validators.ValidatorType {
	return validators.ECDSAValidatorType
}

// Address returns the address of KeyManager
func (s *ECDSAKeyManager) Address() types.Address {
	return s.address
}

// NewEmptyValidators returns empty validator collection ECDSAKeyManager uses
func (s *ECDSAKeyManager) NewEmptyValidators() validators.Validators {
	return validators.NewECDSAValidatorSet()
}

// NewEmptyCommittedSeals returns empty CommittedSeals ECDSAKeyManager uses
func (s *ECDSAKeyManager) NewEmptyCommittedSeals() Seals {
	return &SerializedSeal{}
}

// SignProposerSeal signs the given message by ECDSA key the ECDSAKeyManager holds for ProposerSeal
func (s *ECDSAKeyManager) SignProposerSeal(message []byte) ([]byte, error) {
	return crypto.Sign(s.key, message)
}

// SignProposerSeal signs the given message by ECDSA key the ECDSAKeyManager holds for committed seal
func (s *ECDSAKeyManager) SignCommittedSeal(message []byte) ([]byte, error) {
	return crypto.Sign(s.key, message)
}

// VerifyCommittedSeal verifies a committed seal
func (s *ECDSAKeyManager) VerifyCommittedSeal(
	vals validators.Validators,
	address types.Address,
	signature []byte,
	message []byte,
) error {
	if vals.Type() != s.Type() {
		return ErrInvalidValidators
	}

	signer, err := s.Ecrecover(signature, message)
	if err != nil {
		return ErrInvalidSignature
	}

	if address != signer {
		return ErrSignerMismatch
	}

	if !vals.Includes(address) {
		return ErrNonValidatorCommittedSeal
	}

	return nil
}

func (s *ECDSAKeyManager) GenerateCommittedSeals(
	sealMap map[types.Address][]byte,
	_ validators.Validators,
) (Seals, error) {
	seals := [][]byte{}

	for _, seal := range sealMap {
		if len(seal) != IstanbulExtraSeal {
			return nil, ErrInvalidCommittedSealLength
		}

		seals = append(seals, seal)
	}

	serializedSeal := SerializedSeal(seals)

	return &serializedSeal, nil
}

func (s *ECDSAKeyManager) VerifyCommittedSeals(
	rawCommittedSeal Seals,
	digest []byte,
	vals validators.Validators,
) (int, error) {
	committedSeal, ok := rawCommittedSeal.(*SerializedSeal)
	if !ok {
		return 0, ErrInvalidCommittedSealType
	}

	if vals.Type() != s.Type() {
		return 0, ErrInvalidValidators
	}

	return s.verifyCommittedSealsImpl(committedSeal, digest, vals)
}

func (s *ECDSAKeyManager) SignIBFTMessage(msg []byte) ([]byte, error) {
	return crypto.Sign(s.key, msg)
}

func (s *ECDSAKeyManager) Ecrecover(sig, digest []byte) (types.Address, error) {
	return ecrecover(sig, digest)
}

func (s *ECDSAKeyManager) verifyCommittedSealsImpl(
	committedSeal *SerializedSeal,
	msg []byte,
	validators validators.Validators,
) (int, error) {
	numSeals := committedSeal.Num()
	if numSeals == 0 {
		return 0, ErrEmptyCommittedSeals
	}

	visited := make(map[types.Address]bool)

	for _, seal := range *committedSeal {
		addr, err := s.Ecrecover(seal, msg)
		if err != nil {
			return 0, err
		}

		if visited[addr] {
			return 0, ErrRepeatedCommittedSeal
		}

		if !validators.Includes(addr) {
			return 0, ErrNonValidatorCommittedSeal
		}

		visited[addr] = true
	}

	return numSeals, nil
}

type SerializedSeal [][]byte

func (s *SerializedSeal) Num() int {
	return len(*s)
}

func (s *SerializedSeal) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	if len(*s) == 0 {
		return ar.NewNullArray()
	}

	committed := ar.NewArray()

	for _, a := range *s {
		if len(a) == 0 {
			committed.Set(ar.NewNull())
		} else {
			committed.Set(ar.NewCopyBytes(a))
		}
	}

	return committed
}

func (s *SerializedSeal) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	vals, err := v.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", v.Type())
	}

	(*s) = make([][]byte, len(vals))

	for indx, val := range vals {
		if (*s)[indx], err = val.GetBytes((*s)[indx]); err != nil {
			return err
		}
	}

	return nil
}
