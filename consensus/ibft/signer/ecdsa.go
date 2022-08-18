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

type ECDSAKeyManager struct {
	key     *ecdsa.PrivateKey
	address types.Address
}

type SerializedSeal [][]byte

func NewECDSAKeyManager(manager secrets.SecretsManager) (KeyManager, error) {
	key, err := getOrCreateECDSAKey(manager)
	if err != nil {
		return nil, err
	}

	return NewECDSAKeyManagerFromKey(key), nil
}

func NewECDSAKeyManagerFromKey(key *ecdsa.PrivateKey) KeyManager {
	return &ECDSAKeyManager{
		key:     key,
		address: crypto.PubKeyToAddress(&key.PublicKey),
	}
}

func NewMockECDSAKeyManager(addr types.Address) KeyManager {
	return &ECDSAKeyManager{
		address: addr,
	}
}

func (s *ECDSAKeyManager) Type() validators.ValidatorType {
	return validators.ECDSAValidatorType
}

func (s *ECDSAKeyManager) Address() types.Address {
	return s.address
}

func (s *ECDSAKeyManager) NewEmptyValidatorSet() validators.Validators {
	return &validators.ECDSAValidators{}
}

func (s *ECDSAKeyManager) NewEmptyCommittedSeals() Sealer {
	return &SerializedSeal{}
}

func (s *ECDSAKeyManager) SignSeal(data []byte) ([]byte, error) {
	return crypto.Sign(s.key, data)
}

func (s *ECDSAKeyManager) SignCommittedSeal(data []byte) ([]byte, error) {
	return crypto.Sign(s.key, crypto.Keccak256(data))
}

func (s *ECDSAKeyManager) Ecrecover(sig, digest []byte) (types.Address, error) {
	return ecrecover(sig, digest)
}

func (s *ECDSAKeyManager) GenerateCommittedSeals(
	sealMap map[types.Address][]byte,
	extra *IstanbulExtra,
) (Sealer, error) {
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

func (s *ECDSAKeyManager) VerifyCommittedSeal(
	rawSet validators.Validators,
	addr types.Address,
	signature []byte,
	hash []byte,
) error {
	validatorSet, ok := rawSet.(*validators.ECDSAValidators)
	if !ok {
		return ErrInvalidValidatorSet
	}

	signer, err := s.Ecrecover(signature, hash)
	if err != nil {
		return ErrInvalidSignature
	}

	if addr != signer {
		return ErrSignerMismatch
	}

	if !validatorSet.Includes(addr) {
		return ErrNonValidatorCommittedSeal
	}

	return nil
}

func (s *ECDSAKeyManager) VerifyCommittedSeals(
	rawCommittedSeal Sealer,
	digest []byte,
	rawSet validators.Validators,
) (int, error) {
	committedSeal, ok := rawCommittedSeal.(*SerializedSeal)
	if !ok {
		return 0, ErrInvalidCommittedSealType
	}

	validatorSet, ok := rawSet.(*validators.ECDSAValidators)
	if !ok {
		return 0, ErrInvalidValidatorSet
	}

	return s.verifyCommittedSealsImpl(committedSeal, digest, *validatorSet)
}

func (s *ECDSAKeyManager) SignIBFTMessage(msg []byte) ([]byte, error) {
	return crypto.Sign(s.key, crypto.Keccak256(msg))
}

func (s *ECDSAKeyManager) verifyCommittedSealsImpl(
	committedSeal *SerializedSeal,
	msg []byte,
	validators validators.ECDSAValidators,
) (int, error) {
	numSeals := len(*committedSeal)
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
