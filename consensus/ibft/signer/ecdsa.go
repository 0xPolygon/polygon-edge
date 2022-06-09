package signer

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ECDSAKeyManager struct {
	key     *ecdsa.PrivateKey
	address types.Address
}

type SerializedSeal [][]byte

func NewECDSAKeyManager(manager secrets.SecretsManager) (KeyManager, error) {
	key, err := obtainOrCreateECDSAKey(manager)
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

func (s *ECDSAKeyManager) Address() types.Address {
	return s.address
}

func (s *ECDSAKeyManager) NewEmptyIstanbulExtra() *IstanbulExtra {
	return &IstanbulExtra{
		Validators:          &validators.ECDSAValidatorSet{},
		CommittedSeal:       &SerializedSeal{},
		ParentCommittedSeal: &SerializedSeal{},
	}
}

func (s *ECDSAKeyManager) NewEmptyCommittedSeal() Sealer {
	return &SerializedSeal{}
}

func (s *ECDSAKeyManager) SignSeal(data []byte) ([]byte, error) {
	return crypto.Sign(s.key, data)
}

func (s *ECDSAKeyManager) SignCommittedSeal(data []byte) ([]byte, error) {
	return crypto.Sign(s.key, data)
}

func (s *ECDSAKeyManager) Ecrecover(sig, digest []byte) (types.Address, error) {
	return ecrecoverImpl(sig, digest)
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
	rawCommittedSeal Sealer,
	digest []byte,
	rawSet validators.ValidatorSet,
) (int, error) {
	committedSeal, ok := rawCommittedSeal.(*SerializedSeal)
	if !ok {
		return 0, ErrInvalidCommittedSealType
	}

	validatorSet, ok := rawSet.(*validators.ECDSAValidatorSet)
	if !ok {
		return 0, ErrInvalidValidatorSet
	}

	return s.verifyCommittedSealsImpl(committedSeal, digest, *validatorSet)
}

func (s *ECDSAKeyManager) SignIBFTMessage(msg *proto.MessageReq) error {
	return signMsg(s.key, msg)
}

func (s *ECDSAKeyManager) ValidateIBFTMessage(msg *proto.MessageReq) error {
	return ValidateMsg(msg)
}

func (s *ECDSAKeyManager) verifyCommittedSealsImpl(
	committedSeal *SerializedSeal,
	msg []byte,
	validators validators.ECDSAValidatorSet,
) (int, error) {
	numSeals := len(*committedSeal)
	if numSeals == 0 {
		return 0, ErrEmptyCommittedSeals
	}

	visited := map[types.Address]struct{}{}

	for _, seal := range *committedSeal {
		addr, err := s.Ecrecover(seal, msg)
		if err != nil {
			return 0, err
		}

		if _, ok := visited[addr]; ok {
			return 0, ErrRepeatedCommittedSeal
		}

		if !validators.Includes(addr) {
			return 0, ErrNonValidatorCommittedSeal
		}

		visited[addr] = struct{}{}
	}

	return numSeals, nil
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
			committed.Set(ar.NewBytes(a))
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
