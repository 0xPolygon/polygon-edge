package polybft

import (
	"fmt"
	"math/big"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
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
	Validators  *validator.ValidatorSetDelta
	Parent      *Signature
	Committed   *Signature
	Checkpoint  *CheckpointData
	Dummy1      string // MyFirstFork fork
	Dummy2      string // MySecondFork fork
	BlockNumber uint64 // field used by forking manager
}

// MarshalRLPTo defines the marshal function wrapper for Extra
func (e *Extra) MarshalRLPTo(dst []byte) []byte {
	ar := &fastrlp.Arena{}

	return append(make([]byte, ExtraVanity), e.MarshalRLPWith(ar).MarshalTo(dst)...)
}

// MarshalRLPWith defines the marshal function implementation for Extra
func (e *Extra) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	return GetExtraHandler(e.BlockNumber).MarshalRLPWith(e, ar)
}

// UnmarshalRLP defines the unmarshal function wrapper for Extra
func (e *Extra) UnmarshalRLP(input []byte) error {
	return fastrlp.UnmarshalRLP(input[ExtraVanity:], e)
}

// UnmarshalRLPWith defines the unmarshal implementation for Extra
func (e *Extra) UnmarshalRLPWith(v *fastrlp.Value) error {
	return GetExtraHandler(e.BlockNumber).UnmarshalRLPWith(e, v)
}

// ValidateFinalizedData contains extra data validations for finalized headers
func (e *Extra) ValidateFinalizedData(header *types.Header, parent *types.Header, parents []*types.Header,
	chainID uint64, consensusBackend polybftBackend, domain []byte, logger hclog.Logger) error {
	// validate committed signatures
	blockNumber := header.Number
	if e.Committed == nil {
		return fmt.Errorf("failed to verify signatures for block %d, because signatures are not present", blockNumber)
	}

	if e.Checkpoint == nil {
		return fmt.Errorf("failed to verify signatures for block %d, because checkpoint data are not present", blockNumber)
	}

	// validate current block signatures
	checkpointHash, err := e.Checkpoint.Hash(chainID, header.Number, header.Hash)
	if err != nil {
		return fmt.Errorf("failed to calculate proposal hash: %w", err)
	}

	validators, err := consensusBackend.GetValidators(blockNumber-1, parents)
	if err != nil {
		return fmt.Errorf("failed to validate header for block %d. could not retrieve block validators:%w", blockNumber, err)
	}

	if err := e.Committed.Verify(validators, checkpointHash, domain, logger); err != nil {
		return fmt.Errorf("failed to verify signatures for block %d (proposal hash %s): %w",
			blockNumber, checkpointHash, err)
	}

	parentExtra, err := GetIbftExtra(parent.ExtraData, parent.Number)
	if err != nil {
		return fmt.Errorf("failed to verify signatures for block %d: %w", blockNumber, err)
	}

	// validate parent signatures
	if err := e.ValidateParentSignatures(blockNumber, consensusBackend, parents,
		parent, parentExtra, chainID, domain, logger); err != nil {
		return err
	}

	return e.Checkpoint.ValidateBasic(parentExtra.Checkpoint)
}

// ValidateParentSignatures validates signatures for parent block
func (e *Extra) ValidateParentSignatures(blockNumber uint64, consensusBackend polybftBackend, parents []*types.Header,
	parent *types.Header, parentExtra *Extra, chainID uint64, domain []byte, logger hclog.Logger) error {
	// skip block 1 because genesis does not have committed signatures
	if blockNumber <= 1 {
		return nil
	}

	if e.Parent == nil {
		return fmt.Errorf("failed to verify signatures for parent of block %d because signatures are not present",
			blockNumber)
	}

	parentValidators, err := consensusBackend.GetValidators(blockNumber-2, parents)
	if err != nil {
		return fmt.Errorf(
			"failed to validate header for block %d. could not retrieve parent validators: %w",
			blockNumber,
			err,
		)
	}

	parentCheckpointHash, err := parentExtra.Checkpoint.Hash(chainID, parent.Number, parent.Hash)
	if err != nil {
		return fmt.Errorf("failed to calculate parent proposal hash: %w", err)
	}

	if err := e.Parent.Verify(parentValidators, parentCheckpointHash, domain, logger); err != nil {
		return fmt.Errorf("failed to verify signatures for parent of block %d (proposal hash: %s): %w",
			blockNumber, parentCheckpointHash, err)
	}

	return nil
}

func (e *Extra) ValidateAdditional(header *types.Header) error {
	return GetExtraHandler(e.BlockNumber).ValidateAdditional(e, header)
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

// Verify is used to verify aggregated signature based on current validator set, message hash and domain
func (s *Signature) Verify(validators validator.AccountSet, hash types.Hash, domain []byte, logger hclog.Logger) error {
	signers, err := validators.GetFilteredValidators(s.Bitmap)
	if err != nil {
		return err
	}

	validatorSet := validator.NewValidatorSet(validators, logger)
	if !validatorSet.HasQuorum(signers.GetAddressesAsSet()) {
		return fmt.Errorf("quorum not reached")
	}

	blsPublicKeys := make([]*bls.PublicKey, len(signers))
	for i, validator := range signers {
		blsPublicKeys[i] = validator.BlsKey
	}

	aggs, err := bls.UnmarshalSignature(s.AggregatedSignature)
	if err != nil {
		return err
	}

	if !aggs.VerifyAggregated(blsPublicKeys, hash[:], domain) {
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

// ValidateBasic encapsulates basic validation logic for checkpoint data.
// It only checks epoch numbers validity and whether validators hashes are non-empty.
func (c *CheckpointData) ValidateBasic(parentCheckpoint *CheckpointData) error {
	if c.EpochNumber != parentCheckpoint.EpochNumber &&
		c.EpochNumber != parentCheckpoint.EpochNumber+1 {
		// epoch-beginning block
		// epoch number must be incremented by one compared to parent block's checkpoint
		return fmt.Errorf("invalid epoch number for epoch-beginning block")
	}

	if c.CurrentValidatorsHash == types.ZeroHash {
		return fmt.Errorf("current validators hash must not be empty")
	}

	if c.NextValidatorsHash == types.ZeroHash {
		return fmt.Errorf("next validators hash must not be empty")
	}

	return nil
}

// Validate encapsulates validation logic for checkpoint data
// (with regards to current and next epoch validators)
func (c *CheckpointData) Validate(parentCheckpoint *CheckpointData,
	currentValidators validator.AccountSet, nextValidators validator.AccountSet) error {
	if err := c.ValidateBasic(parentCheckpoint); err != nil {
		return err
	}

	// check if currentValidatorsHash, present in CheckpointData is correct
	currentValidatorsHash, err := currentValidators.Hash()
	if err != nil {
		return fmt.Errorf("failed to calculate current validators hash: %w", err)
	}

	if currentValidatorsHash != c.CurrentValidatorsHash {
		return fmt.Errorf("current validators hashes don't match")
	}

	// check if nextValidatorsHash, present in CheckpointData is correct
	nextValidatorsHash, err := nextValidators.Hash()
	if err != nil {
		return fmt.Errorf("failed to calculate next validators hash: %w", err)
	}

	if nextValidatorsHash != c.NextValidatorsHash {
		return fmt.Errorf("next validators hashes don't match")
	}

	// epoch ending blocks have validator set transitions
	if !currentValidators.Equals(nextValidators) &&
		c.EpochNumber != parentCheckpoint.EpochNumber {
		// epoch ending blocks should have the same epoch number as parent block
		// (as they belong to the same epoch)
		return fmt.Errorf("epoch number should not change for epoch-ending block")
	}

	return nil
}

// GetIbftExtraClean returns unmarshaled extra field from the passed in header,
// but without signatures for the given header (it only includes signatures for the parent block)
func GetIbftExtraClean(extraRaw []byte, blockNumber uint64) ([]byte, error) {
	extra, err := GetIbftExtra(extraRaw, blockNumber)
	if err != nil {
		return nil, err
	}

	return GetExtraHandler(blockNumber).GetIbftExtraClean(extra).MarshalRLPTo(nil), nil
}

// GetIbftExtra returns the istanbul extra data field from the passed in header
func GetIbftExtra(extraRaw []byte, blockNumber uint64) (*Extra, error) {
	if len(extraRaw) < ExtraVanity {
		return nil, fmt.Errorf("wrong extra size: %d", len(extraRaw))
	}

	extra := &Extra{
		BlockNumber: blockNumber,
	}

	if err := extra.UnmarshalRLP(extraRaw); err != nil {
		return nil, err
	}

	if extra.Validators == nil {
		extra.Validators = &validator.ValidatorSetDelta{}
	}

	return extra, nil
}
