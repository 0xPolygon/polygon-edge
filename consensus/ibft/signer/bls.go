package signer

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/validators"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/coinbase/kryptology/pkg/signatures/bls/bls_sig"
	"github.com/umbracle/fastrlp"
)

type BLSSigner struct {
	ecdsaKey *ecdsa.PrivateKey
	blsKey   *bls_sig.SecretKey
	address  types.Address
}

func NewBLSSigner(manager secrets.SecretsManager) (Signer, error) {
	ecdsaKey, err := obtainOrCreateECDSAKey(manager)
	if err != nil {
		return nil, err
	}

	blsKey, err := crypto.ECDSAToBLS(ecdsaKey)
	if err != nil {
		return nil, err
	}

	return &BLSSigner{
		ecdsaKey: ecdsaKey,
		blsKey:   blsKey,
		address:  crypto.PubKeyToAddress(&ecdsaKey.PublicKey),
	}, nil
}

func (s *BLSSigner) Address() types.Address {
	return s.address
}

func (s *BLSSigner) InitIBFTExtra(header, parent *types.Header, set validators.ValidatorSet) error {
	var parentCommittedSeal Sealer

	if parent.Number >= 1 {
		parentExtra, err := s.GetIBFTExtra(parent)
		if err != nil {
			return err
		}

		parentCommittedSeal = parentExtra.CommittedSeal
	}

	s.initIbftExtra(header, set, parentCommittedSeal)

	return nil
}

func (s *BLSSigner) GetIBFTExtra(h *types.Header) (*IstanbulExtra, error) {
	if len(h.ExtraData) < IstanbulExtraVanity {
		return nil, fmt.Errorf(
			"wrong extra size, expected greater than or equal to %d but actual %d",
			IstanbulExtraVanity,
			len(h.ExtraData),
		)
	}

	data := h.ExtraData[IstanbulExtraVanity:]
	extra := &IstanbulExtra{
		Validators:          &validators.BLSValidatorSet{},
		CommittedSeal:       &BLSSeal{},
		ParentCommittedSeal: &BLSSeal{},
	}

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	return extra, nil
}

func (s *BLSSigner) WriteSeal(header *types.Header) (*types.Header, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	seal, err := crypto.Sign(s.ecdsaKey, crypto.Keccak256(hash[:]))
	if err != nil {
		return nil, err
	}

	err = s.packFieldIntoIbftExtra(header, func(ie *IstanbulExtra) {
		ie.Seal = seal
	})

	if err != nil {
		return nil, err
	}

	return header, nil
}

func (s *BLSSigner) EcrecoverFromHeader(header *types.Header) (types.Address, error) {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return types.Address{}, err
	}

	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return types.Address{}, err
	}

	return ecrecoverImpl(extra.Seal, hash[:])
}

func (s *BLSSigner) CreateCommittedSeal(header *types.Header) ([]byte, error) {
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return nil, err
	}

	msg := commitMsg(hash[:])

	return signByBLS(s.blsKey, msg)
}

func (s *BLSSigner) WriteCommittedSeals(header *types.Header, sealMap map[types.Address][]byte) (*types.Header, error) {
	if len(sealMap) == 0 {
		return nil, ErrEmptyCommittedSeals
	}

	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return nil, err
	}

	validators, ok := extra.Validators.(*validators.BLSValidatorSet)
	if !ok {
		return nil, ErrInvalidValidatorSet
	}

	blsPop := bls_sig.NewSigPop()

	bitMap := new(big.Int)
	blsSignatures := make([]*bls_sig.Signature, 0, len(sealMap))

	for addr, seal := range sealMap {
		index := validators.Index(addr)
		if index == -1 {
			return nil, ErrNonValidatorCommittedSeal
		}

		bitMap = bitMap.SetBit(bitMap, index, 1)

		bsig := &bls_sig.Signature{}
		if err := bsig.UnmarshalBinary(seal); err != nil {
			return nil, err
		}

		blsSignatures = append(blsSignatures, bsig)
	}

	multiSignature, err := blsPop.AggregateSignatures(blsSignatures...)
	if err != nil {
		return nil, err
	}

	multiSignatureBytes, err := multiSignature.MarshalBinary()
	if err != nil {
		return nil, err
	}

	blsSeal := BLSSeal{
		Bitmap:    bitMap,
		Signature: multiSignatureBytes,
	}

	err = s.packFieldIntoIbftExtra(header, func(ie *IstanbulExtra) {
		ie.CommittedSeal = &blsSeal
	})

	if err != nil {
		return nil, err
	}

	return header, nil
}

func (s *BLSSigner) VerifyCommittedSeal(
	rawSet validators.ValidatorSet,
	header *types.Header,
	quorumFn validators.QuorumImplementation,
) error {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	cs, ok := extra.CommittedSeal.(*BLSSeal)
	if !ok {
		return ErrInvalidCommittedSealType
	}

	validatorSet, ok := rawSet.(*validators.BLSValidatorSet)
	if !ok {
		return ErrInvalidValidatorSet
	}

	// get the message that needs to be signed
	// this not signing! just removing the fields that should be signed
	hash, err := s.CalculateHeaderHash(header)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash[:])

	return s.verifyCommittedSealsImpl(cs, rawMsg, *validatorSet, quorumFn)
}

func (s *BLSSigner) VerifyParentCommittedSeal(
	rawParentSet validators.ValidatorSet,
	parent, header *types.Header,
	quorumFn validators.QuorumImplementation,
) error {
	extra, err := s.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	parentCs, ok := extra.ParentCommittedSeal.(*BLSSeal)
	if !ok {
		return ErrInvalidCommittedSealType
	}

	validatorSet, ok := rawParentSet.(*validators.BLSValidatorSet)
	if !ok {
		return ErrInvalidValidatorSet
	}

	// get the message that needs to be signed
	// this not signing! just removing the fields that should be signed
	hash, err := s.CalculateHeaderHash(parent)
	if err != nil {
		return err
	}

	rawMsg := commitMsg(hash[:])

	return s.verifyCommittedSealsImpl(parentCs, rawMsg, *validatorSet, quorumFn)
}

func (s *BLSSigner) SignGossipMessage(msg *proto.MessageReq) error {
	return signMsg(s.ecdsaKey, msg)
}

func (s *BLSSigner) ValidateGossipMessage(msg *proto.MessageReq) error {
	return ValidateMsg(msg)
}

func (s *BLSSigner) initIbftExtra(header *types.Header, vals validators.ValidatorSet, parentCommittedSeal Sealer) {
	putIbftExtra(header, &IstanbulExtra{
		Validators:          vals,
		Seal:                nil,
		CommittedSeal:       &BLSSeal{},
		ParentCommittedSeal: parentCommittedSeal,
	})
}

func (s *BLSSigner) packFieldIntoIbftExtra(h *types.Header, updateFn func(*IstanbulExtra)) error {
	extra, err := s.GetIBFTExtra(h)
	if err != nil {
		return err
	}

	updateFn(extra)

	putIbftExtra(h, extra)

	return nil
}

func (s *BLSSigner) CalculateHeaderHash(header *types.Header) (types.Hash, error) {
	filteredHeader, err := s.filterHeaderForHash(header)
	if err != nil {
		return types.ZeroHash, err
	}

	return calculateHeaderHash(filteredHeader), nil
}

func (s *BLSSigner) filterHeaderForHash(header *types.Header) (*types.Header, error) {
	// This will effectively remove the Seal and Committed Seal fields,
	// while keeping proposer vanity and validator set
	// because extra.Validators, extra.ParentCommittedSeal is what we got from `h` in the first place.
	clone := header.Copy()

	extra, err := s.GetIBFTExtra(clone)
	if err != nil {
		return nil, err
	}

	s.initIbftExtra(clone, extra.Validators, extra.ParentCommittedSeal)

	return clone, nil
}

func (s *BLSSigner) verifyCommittedSealsImpl(
	committedSeal *BLSSeal,
	msg []byte,
	validators validators.BLSValidatorSet,
	quorumFn validators.QuorumImplementation,
) error {
	if len(committedSeal.Signature) == 0 || committedSeal.Bitmap.BitLen() == 0 {
		return ErrEmptyCommittedSeals
	}

	blsPop := bls_sig.NewSigPop()
	pubkeys := make([]*bls_sig.PublicKey, 0, validators.Len())

	for idx, val := range validators {
		if committedSeal.Bitmap.Bit(idx) == 0 {
			continue
		}

		pubKey := &bls_sig.PublicKey{}
		if err := pubKey.UnmarshalBinary(val.BLSPubKey); err != nil {
			return err
		}

		pubkeys = append(pubkeys, pubKey)
	}

	multiPubKey, err := blsPop.AggregatePublicKeys(pubkeys...)
	if err != nil {
		return err
	}

	signature := &bls_sig.MultiSignature{}
	if err := signature.UnmarshalBinary(committedSeal.Signature); err != nil {
		return err
	}

	ok, err := blsPop.VerifyMultiSignature(multiPubKey, msg, signature)
	if err != nil {
		return err
	}

	if !ok {
		return ErrInvalidBLSSignature
	}

	// Valid committed seals must be at least 2F+1
	// 	2F 	is the required number of honest validators who provided the committed seals
	// 	+1	is the proposer
	if validSeals := len(pubkeys); validSeals < quorumFn(&validators) {
		return ErrNotEnoughCommittedSeals
	}

	return nil
}

type BLSSeal struct {
	Bitmap    *big.Int
	Signature []byte
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
		x.Set(ar.NewBytes(s.Signature))
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

	if len(vals) != 2 {
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

func signByBLS(prv *bls_sig.SecretKey, msg []byte) ([]byte, error) {
	blsPop := bls_sig.NewSigPop()
	seal, err := blsPop.Sign(prv, msg)

	if err != nil {
		return nil, err
	}

	sealBytes, err := seal.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return sealBytes, nil
}

// func signMsgByBLS(key *bls_sig.SecretKey, msg *proto.MessageReq) error {
// 	signMsg, err := msg.PayloadNoSig()
// 	if err != nil {
// 		return err
// 	}

// 	blsPop := bls_sig.NewSigPop()
// 	sig, err := blsPop.Sign(key, crypto.Keccak256(signMsg))
// 	if err != nil {
// 		return err
// 	}

// 	sigBytes, err := sig.MarshalBinary()
// 	if err != nil {
// 		return err
// 	}

// 	msg.Signature = hex.EncodeToHex(sigBytes)

// 	return nil
// }
