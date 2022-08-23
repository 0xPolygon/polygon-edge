package signer

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
	"github.com/umbracle/fastrlp"
)

// BLSKeyManager is a module that holds ECDSA and BLS keys
// and implements methods of signing by these keys
type BLSKeyManager struct {
	ecdsaKey *ecdsa.PrivateKey
	blsKey   *bls_sig.SecretKey
	address  types.Address
}

// NewBLSKeyManager initializes BLSKeyManager by the ECDSA key and BLS key which are loaded from SecretsManager
func NewBLSKeyManager(manager secrets.SecretsManager) (KeyManager, error) {
	ecdsaKey, err := getOrCreateECDSAKey(manager)
	if err != nil {
		return nil, err
	}

	blsKey, err := getOrCreateBLSKey(manager)
	if err != nil {
		return nil, err
	}

	return NewBLSKeyManagerFromKeys(ecdsaKey, blsKey), nil
}

// NewBLSKeyManagerFromKeys initializes BLSKeyManager from the given ECDSA and BLS keys
func NewBLSKeyManagerFromKeys(ecdsaKey *ecdsa.PrivateKey, blsKey *bls_sig.SecretKey) KeyManager {
	return &BLSKeyManager{
		ecdsaKey: ecdsaKey,
		blsKey:   blsKey,
		address:  crypto.PubKeyToAddress(&ecdsaKey.PublicKey),
	}
}

// Type returns the validator type KeyManager supports
func (s *BLSKeyManager) Type() validators.ValidatorType {
	return validators.BLSValidatorType
}

// Address returns the address of KeyManager
func (s *BLSKeyManager) Address() types.Address {
	return s.address
}

// NewEmptyValidators returns empty validator collection BLSKeyManager uses
func (s *BLSKeyManager) NewEmptyValidators() validators.Validators {
	return &validators.BLSValidators{}
}

// NewEmptyCommittedSeals returns empty CommittedSeals BLSKeyManager uses
func (s *BLSKeyManager) NewEmptyCommittedSeals() Sealer {
	return &BLSSeal{}
}

func (s *BLSKeyManager) SignProposerSeal(data []byte) ([]byte, error) {
	return crypto.Sign(s.ecdsaKey, data)
}

func (s *BLSKeyManager) SignCommittedSeal(data []byte) ([]byte, error) {
	return crypto.SignByBLS(s.blsKey, data)
}

func (s *BLSKeyManager) VerifyCommittedSeal(
	rawSet validators.Validators,
	addr types.Address,
	rawSignature []byte,
	hash []byte,
) error {
	validatorSet, ok := rawSet.(*validators.BLSValidators)
	if !ok {
		return ErrInvalidValidators
	}

	validatorIndex := validatorSet.Index(addr)
	if validatorIndex == -1 {
		return ErrValidatorNotFound
	}

	validator, _ := validatorSet.At(uint64(validatorIndex)).(*validators.BLSValidator)

	if err := crypto.VerifyBLSSignatureFromBytes(
		validator.BLSPublicKey,
		rawSignature,
		hash,
	); err != nil {
		return err
	}

	return nil
}

func (s *BLSKeyManager) GenerateCommittedSeals(
	sealMap map[types.Address][]byte,
	rawValidators validators.Validators,
) (Sealer, error) {
	validators, ok := rawValidators.(*validators.BLSValidators)
	if !ok {
		return nil, ErrInvalidValidators
	}

	blsSignatures, bitMap, err := getBLSSignatures(sealMap, validators)
	if err != nil {
		return nil, err
	}

	multiSignature, err := bls_sig.NewSigPop().AggregateSignatures(blsSignatures...)
	if err != nil {
		return nil, err
	}

	multiSignatureBytes, err := multiSignature.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &BLSSeal{
		Bitmap:    bitMap,
		Signature: multiSignatureBytes,
	}, nil
}

func (s *BLSKeyManager) VerifyCommittedSeals(
	rawCommittedSeal Sealer,
	message []byte,
	rawValidators validators.Validators,
) (int, error) {
	committedSeal, ok := rawCommittedSeal.(*BLSSeal)
	if !ok {
		return 0, ErrInvalidCommittedSealType
	}

	validatorSet, ok := rawValidators.(*validators.BLSValidators)
	if !ok {
		return 0, ErrInvalidValidators
	}

	return verifyBLSCommittedSealsImpl(committedSeal, message, *validatorSet)
}

func (s *BLSKeyManager) SignIBFTMessage(msg []byte) ([]byte, error) {
	return crypto.Sign(s.ecdsaKey, msg)
}

func (s *BLSKeyManager) Ecrecover(sig, digest []byte) (types.Address, error) {
	return ecrecover(sig, digest)
}

type BLSSeal struct {
	Bitmap    *big.Int
	Signature []byte
}

func (s *BLSSeal) Num() int {
	return s.Bitmap.BitLen()
}

func (s *BLSSeal) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	x := ar.NewArray()

	if s.Bitmap == nil {
		x.Set(ar.NewNull())
	} else {
		x.Set(ar.NewBytes(s.Bitmap.Bytes()))
	}

	if s.Signature == nil {
		x.Set(ar.NewNull())
	} else {
		x.Set(ar.NewCopyBytes(s.Signature))
	}

	return x
}

func (s *BLSSeal) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	vals, err := v.GetElems()

	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", v.Type())
	}

	if len(vals) == 0 {
		return nil
	}

	if len(vals) < 2 {
		return fmt.Errorf("mismatch of RLP type for AggregatedCommittedSeal")
	}

	var rawBitMap []byte

	rawBitMap, err = vals[0].GetBytes(rawBitMap)
	if err != nil {
		return err
	}

	s.Bitmap = new(big.Int).SetBytes(rawBitMap)

	if s.Signature, err = vals[1].GetBytes(s.Signature); err != nil {
		return err
	}

	return nil
}

func getBLSSignatures(
	sealMap map[types.Address][]byte,
	validators *validators.BLSValidators,
) ([]*bls_sig.Signature, *big.Int, error) {
	blsSignatures := make([]*bls_sig.Signature, 0, len(sealMap))
	bitMap := new(big.Int)

	for addr, seal := range sealMap {
		index := validators.Index(addr)
		if index == -1 {
			return nil, nil, ErrNonValidatorCommittedSeal
		}

		bsig := &bls_sig.Signature{}
		if err := bsig.UnmarshalBinary(seal); err != nil {
			return nil, nil, err
		}

		bitMap = bitMap.SetBit(bitMap, int(index), 1)

		blsSignatures = append(blsSignatures, bsig)
	}

	return blsSignatures, bitMap, nil
}

func createAggregatedBLSPubKeys(
	validators validators.BLSValidators,
	bitMap *big.Int,
) (*bls_sig.MultiPublicKey, int, error) {
	pubkeys := make([]*bls_sig.PublicKey, 0, validators.Len())

	for idx, val := range validators {
		if bitMap.Bit(idx) == 0 {
			continue
		}

		pubKey, err := crypto.UnmarshalBLSPublicKey(val.BLSPublicKey)
		if err != nil {
			return nil, 0, err
		}

		pubkeys = append(pubkeys, pubKey)
	}

	key, err := bls_sig.NewSigPop().AggregatePublicKeys(pubkeys...)
	if err != nil {
		return nil, 0, err
	}

	return key, len(pubkeys), nil
}

func verifyBLSCommittedSealsImpl(
	committedSeal *BLSSeal,
	msg []byte,
	validators validators.BLSValidators,
) (int, error) {
	if len(committedSeal.Signature) == 0 ||
		committedSeal.Bitmap == nil ||
		committedSeal.Bitmap.BitLen() == 0 {
		return 0, ErrEmptyCommittedSeals
	}

	aggregatedPubKey, numKeys, err := createAggregatedBLSPubKeys(validators, committedSeal.Bitmap)
	if err != nil {
		return 0, fmt.Errorf("failed to aggregate BLS Public Keys: %w", err)
	}

	signature := &bls_sig.MultiSignature{}
	if err := signature.UnmarshalBinary(committedSeal.Signature); err != nil {
		return 0, err
	}

	ok, err := bls_sig.NewSigPop().VerifyMultiSignature(aggregatedPubKey, msg, signature)
	if err != nil {
		return 0, err
	}

	if !ok {
		return 0, ErrInvalidSignature
	}

	return numKeys, nil
}
