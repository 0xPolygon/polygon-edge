package polybft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/fastrlp"
)

const (
	// ExtraVanity represents a fixed number of extra-data bytes reserved for proposer vanity
	ExtraVanity = 32

	// ExtraSeal represents the fixed number of extra-data bytes reserved for proposer seal
	ExtraSeal = 65
)

// PolyBFTMixDigest represents a hash of "PolyBFT Mix" to identify whether the block is from PolyBFT consensus engine
var PolyBFTMixDigest = types.StringToHash("adce6e5230abe012342a44e4e9b6d05997d6f015387ae0e59be924afc7ec70c1")

// Extra defines the structure of the extra field for Istanbul
type Extra struct {
	Validators *ValidatorSetDelta
	Seal       []byte
	Parent     *Signature
	Committed  *Signature
	Checkpoint *CheckpointData
}

// MarshalRLPTo defines the marshal function wrapper for Extra
func (i *Extra) MarshalRLPTo(dst []byte) []byte {
	ar := &fastrlp.Arena{}

	return i.MarshalRLPWith(ar).MarshalTo(dst)
}

// MarshalRLPWith defines the marshal function implementation for Extra
func (i *Extra) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()

	// Validators
	if i.Validators == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(i.Validators.MarshalRLPWith(ar))
	}

	// Seal
	if len(i.Seal) == 0 {
		vv.Set(ar.NewNull())
	} else {
		vv.Set(ar.NewBytes(i.Seal))
	}

	// ParentSeal
	if i.Parent == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(i.Parent.MarshalRLPWith(ar))
	}

	// CommittedSeal
	if i.Committed == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(i.Committed.MarshalRLPWith(ar))
	}

	// Checkpoint
	if i.Checkpoint == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(i.Checkpoint.MarshalRLPWith(ar))
	}

	return vv
}

// UnmarshalRLP defines the unmarshal function wrapper for Extra
func (i *Extra) UnmarshalRLP(input []byte) error {
	return fastrlp.UnmarshalRLP(input, i)
}

// UnmarshalRLPWith defines the unmarshal implementation for Extra
func (i *Extra) UnmarshalRLPWith(v *fastrlp.Value) error {
	const expectedElements = 5

	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != expectedElements {
		return fmt.Errorf("incorrect elements count to decode Extra, expected %d but found %d", expectedElements, num)
	}

	// Validators
	if elems[0].Elems() > 0 {
		i.Validators = &ValidatorSetDelta{}
		if err := i.Validators.UnmarshalRLPWith(elems[0]); err != nil {
			return err
		}
	}

	// Seal
	if elems[1].Len() > 0 {
		if i.Seal, err = elems[1].GetBytes(i.Seal); err != nil {
			return err
		}
	}

	// Parent
	if elems[2].Elems() > 0 {
		i.Parent = &Signature{}
		if err := i.Parent.UnmarshalRLPWith(elems[2]); err != nil {
			return err
		}
	}

	// Committed
	if elems[3].Elems() > 0 {
		i.Committed = &Signature{}
		if err := i.Committed.UnmarshalRLPWith(elems[3]); err != nil {
			return err
		}
	}

	// Checkpoint
	if elems[4].Elems() > 0 {
		i.Checkpoint = &CheckpointData{}
		if err := i.Checkpoint.UnmarshalRLPWith(elems[4]); err != nil {
			return err
		}
	}

	return nil
}

// createValidatorSetDelta calculates ValidatorSetDelta based on the provided old and new validator sets
func createValidatorSetDelta(oldValidatorSet, newValidatorSet AccountSet) (*ValidatorSetDelta, error) {
	var addedValidators, updatedValidators AccountSet

	oldValidatorSetMap := make(map[types.Address]*ValidatorMetadata)
	removedValidators := map[types.Address]int{}

	for i, validator := range oldValidatorSet {
		if (validator.Address != types.Address{}) {
			removedValidators[validator.Address] = i
			oldValidatorSetMap[validator.Address] = validator
		}
	}

	for _, newValidator := range newValidatorSet {
		// Check if the validator is among both old and new validator set
		oldValidator, validatorExists := oldValidatorSetMap[newValidator.Address]
		if validatorExists {
			if !oldValidator.EqualAddressAndBlsKey(newValidator) {
				return nil, fmt.Errorf("validator '%s' found in both old and new validator set, but its BLS keys differ",
					newValidator.Address.String())
			}

			// If it is, then discard it from removed validators...
			delete(removedValidators, newValidator.Address)

			if !oldValidator.Equals(newValidator) {
				updatedValidators = append(updatedValidators, newValidator)
			}
		} else {
			// ...otherwise it is added
			addedValidators = append(addedValidators, newValidator)
		}
	}

	removedValsBitmap := bitmap.Bitmap{}
	for _, i := range removedValidators {
		removedValsBitmap.Set(uint64(i))
	}

	delta := &ValidatorSetDelta{
		Added:   addedValidators,
		Updated: updatedValidators,
		Removed: removedValsBitmap,
	}

	return delta, nil
}

// ValidatorSetDelta holds information about added and removed validators compared to the previous epoch
type ValidatorSetDelta struct {
	// Added is the slice of added validators
	Added AccountSet
	// Updated is the slice of updated valiadtors
	Updated AccountSet
	// Removed is a bitmap of the validators removed from the set
	Removed bitmap.Bitmap
}

// MarshalRLPWith marshals ValidatorSetDelta to RLP format
func (d *ValidatorSetDelta) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	validatorsRaw := ar.NewArray()

	for _, validatorAccount := range d.Added {
		validatorsRaw.Set(validatorAccount.MarshalRLPWith(ar))
	}

	vv.Set(validatorsRaw)              // validators
	vv.Set(ar.NewCopyBytes(d.Removed)) // bitmap

	return vv
}

// UnmarshalRLPWith unmarshals ValidatorSetDelta from RLP format
func (d *ValidatorSetDelta) UnmarshalRLPWith(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) == 0 {
		return nil
	} else if num := len(elems); num != 2 {
		return fmt.Errorf("incorrect elements count to decode validator set delta, expected 2 but found %d", num)
	}

	// Validators (added)
	{
		validatorsRaw, err := elems[0].GetElems()
		if err != nil {
			return fmt.Errorf("array expected for added validators")
		}

		if len(validatorsRaw) != 0 {
			d.Added = make(AccountSet, len(validatorsRaw))

			for i, validatorRaw := range validatorsRaw {
				acc := &ValidatorMetadata{}
				if err = acc.UnmarshalRLPWith(validatorRaw); err != nil {
					return err
				}

				d.Added[i] = acc
			}
		}
	}

	// Bitmap (removed)
	{
		dst, err := elems[1].GetBytes(nil)
		if err != nil {
			return err
		}

		d.Removed = bitmap.Bitmap(dst)
	}

	return nil
}

// IsEmpty returns indication whether delta is empty (namely added, updated slices and removed bitmap are empty)
func (d *ValidatorSetDelta) IsEmpty() bool {
	return d == nil || len(d.Added) == 0 &&
		len(d.Updated) == 0 &&
		d.Removed.Len() == 0
}

// Copy creates deep copy of ValidatorSetDelta
func (d *ValidatorSetDelta) Copy() *ValidatorSetDelta {
	added := d.Added.Copy()
	removed := make([]byte, len(d.Removed))
	copy(removed, d.Removed)

	return &ValidatorSetDelta{Added: added, Removed: removed}
}

// fmt.Stringer interface implementation
func (d *ValidatorSetDelta) String() string {
	return fmt.Sprintf("Added %v Removed %v", d.Added, d.Removed)
}

// Signature represents aggregated signatures of signers accompanied with a bitmap
// (in order to be able to determine identities of each signer)
type Signature struct {
	AggregatedSignature []byte
	Bitmap              []byte
}

// MarshalRLPWith marshals Signature object into RLP format
func (s *Signature) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	committed := ar.NewArray()
	if s.AggregatedSignature == nil {
		committed.Set(ar.NewNull())
	} else {
		committed.Set(ar.NewBytes(s.AggregatedSignature))
	}

	if s.Bitmap == nil {
		committed.Set(ar.NewNull())
	} else {
		committed.Set(ar.NewBytes(s.Bitmap))
	}

	return committed
}

// UnmarshalRLPWith unmarshals Signature object from the RLP format
func (s *Signature) UnmarshalRLPWith(v *fastrlp.Value) error {
	vals, err := v.GetElems()
	if err != nil {
		return fmt.Errorf("array type expected for signature struct")
	}

	// there should be exactly two elements (aggregated signature and bitmap)
	if num := len(vals); num != 2 {
		return fmt.Errorf("incorrect elements count to decode Signature, expected 2 but found %d", num)
	}

	s.AggregatedSignature, err = vals[0].GetBytes(nil)
	if err != nil {
		return err
	}

	s.Bitmap, err = vals[1].GetBytes(nil)
	if err != nil {
		return err
	}

	return nil
}

// VerifyCommittedFields is checking for consensus proof in the header
func (s *Signature) VerifyCommittedFields(validators AccountSet, hash types.Hash) error {
	filtered, err := validators.GetFilteredValidators(s.Bitmap)
	if err != nil {
		return err
	}

	validatorSet, err := NewValidatorSet(validators, hclog.NewNullLogger())
	if err != nil {
		return err
	}

	signerAddresses := filtered.GetAddressesAsSet()
	if !validatorSet.HasQuorum(signerAddresses) {
		return fmt.Errorf("quorum not reached")
	}

	blsPublicKeys := make([]*bls.PublicKey, len(filtered))

	for i, validator := range filtered {
		blsPublicKeys[i] = validator.BlsKey
	}

	// TODO: refactor AggregatedSignature
	aggs, err := bls.UnmarshalSignature(s.AggregatedSignature)
	if err != nil {
		return err
	}

	if !aggs.VerifyAggregated(blsPublicKeys, hash[:]) {
		return fmt.Errorf("could not verify aggregated signature")
	}

	return nil
}

var checkpointDataABIType = abi.MustNewType(`tuple(
	uint256 chainId,
	uint256 blockNumber,
	bytes32 blockHash,
	uint256 blockRound, 
	uint256 epochNumber,
	bytes32 eventRoot,
	bytes32 currentValidatorsHash,
	bytes32 nextValidatorsHash)`)

// CheckpointData represents data needed for checkpointing mechanism
type CheckpointData struct {
	BlockRound            uint64
	EpochNumber           uint64
	CurrentValidatorsHash types.Hash
	NextValidatorsHash    types.Hash
	EventRoot             types.Hash
}

// MarshalRLPWith defines the marshal function implementation for CheckpointData
func (c *CheckpointData) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	// BlockRound
	vv.Set(ar.NewUint(c.BlockRound))
	// EpochNumber
	vv.Set(ar.NewUint(c.EpochNumber))
	// CurrentValidatorsHash
	vv.Set(ar.NewBytes(c.CurrentValidatorsHash.Bytes()))
	// NextValidatorsHash
	vv.Set(ar.NewBytes(c.NextValidatorsHash.Bytes()))
	// EventRoot
	vv.Set(ar.NewBytes(c.EventRoot.Bytes()))

	return vv
}

// UnmarshalRLPWith unmarshals CheckpointData object from the RLP format
func (c *CheckpointData) UnmarshalRLPWith(v *fastrlp.Value) error {
	vals, err := v.GetElems()
	if err != nil {
		return fmt.Errorf("array type expected for CheckpointData struct")
	}

	// there should be exactly 5 elements:
	// BlockRound, EpochNumber, CurrentValidatorsHash, NextValidatorsHash, EventRoot
	if num := len(vals); num != 5 {
		return fmt.Errorf("incorrect elements count to decode CheckpointData, expected 5 but found %d", num)
	}

	// BlockRound
	c.BlockRound, err = vals[0].GetUint64()
	if err != nil {
		return err
	}

	// EpochNumber
	c.EpochNumber, err = vals[1].GetUint64()
	if err != nil {
		return err
	}

	// CurrentValidatorsHash
	currentValidatorsHashRaw, err := vals[2].GetBytes(nil)
	if err != nil {
		return err
	}

	c.CurrentValidatorsHash = types.BytesToHash(currentValidatorsHashRaw)

	// NextValidatorsHash
	nextValidatorsHashRaw, err := vals[3].GetBytes(nil)
	if err != nil {
		return err
	}

	c.NextValidatorsHash = types.BytesToHash(nextValidatorsHashRaw)

	// EventRoot
	eventRootRaw, err := vals[4].GetBytes(nil)
	if err != nil {
		return err
	}

	c.EventRoot = types.BytesToHash(eventRootRaw)

	return nil
}

// Copy returns deep copy of CheckpointData instance
func (c *CheckpointData) Copy() *CheckpointData {
	newCheckpointData := new(CheckpointData)
	*newCheckpointData = *c

	return newCheckpointData
}

// Hash calculates keccak256 hash of the CheckpointData.
// CheckpointData is ABI encoded and then hashed.
func (c *CheckpointData) Hash(chainID uint64, blockNumber uint64, blockHash types.Hash) (types.Hash, error) {
	checkpointMap := map[string]interface{}{
		"chainId":               new(big.Int).SetUint64(chainID),
		"blockNumber":           new(big.Int).SetUint64(blockNumber),
		"blockHash":             blockHash,
		"blockRound":            new(big.Int).SetUint64(c.BlockRound),
		"epochNumber":           new(big.Int).SetUint64(c.EpochNumber),
		"eventRoot":             c.EventRoot,
		"currentValidatorsHash": c.CurrentValidatorsHash,
		"nextValidatorsHash":    c.NextValidatorsHash,
	}

	abiEncoded, err := checkpointDataABIType.Encode(checkpointMap)
	if err != nil {
		return types.ZeroHash, err
	}

	return types.BytesToHash(crypto.Keccak256(abiEncoded)), nil
}

// GetIbftExtraClean returns unmarshaled extra field from the passed in header,
// but without signatures for the given header (it only includes signatures for the parent block)
func GetIbftExtraClean(extraRaw []byte) ([]byte, error) {
	extra, err := GetIbftExtra(extraRaw)
	if err != nil {
		return nil, err
	}

	ibftExtra := &Extra{
		Parent:     extra.Parent,
		Validators: extra.Validators,
		Checkpoint: extra.Checkpoint,
		Seal:       []byte{},
		Committed:  &Signature{},
	}

	return ibftExtra.MarshalRLPTo(nil), nil
}

// GetIbftExtra returns the istanbul extra data field from the passed in header
func GetIbftExtra(extraB []byte) (*Extra, error) {
	if len(extraB) < ExtraVanity {
		return nil, fmt.Errorf("wrong extra size: %d", len(extraB))
	}

	data := extraB[ExtraVanity:]
	extra := &Extra{}

	if err := extra.UnmarshalRLP(data); err != nil {
		return nil, err
	}

	if extra.Validators == nil {
		extra.Validators = &ValidatorSetDelta{}
	}

	return extra, nil
}
