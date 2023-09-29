package contractsapi

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

// StateTransactionInput is an abstraction for different state transaction inputs
type StateTransactionInput interface {
	// EncodeAbi contains logic for encoding arbitrary data into ABI format
	EncodeAbi() ([]byte, error)
	// DecodeAbi contains logic for decoding given ABI data
	DecodeAbi(b []byte) error
}

// EventAbi is an interface representing an event generated in contractsapi
type EventAbi interface {
	// Sig returns the event ABI signature or ID (which is unique for all event types)
	Sig() ethgo.Hash
	// Encode does abi encoding of given event
	Encode() ([]byte, error)
	// ParseLog parses the provided receipt log to given event type
	ParseLog(log *ethgo.Log) (bool, error)
}

var (
	// stateSyncABIType is a specific case where we need to encode state sync event as a tuple of tuple
	stateSyncABIType = abi.MustNewType(
		"tuple(tuple(uint256 id, address sender, address receiver, bytes data))")

	// GetCheckpointBlockABIResponse is the ABI type for getCheckpointBlock function return value
	GetCheckpointBlockABIResponse = abi.MustNewType("tuple(bool isFound, uint256 checkpointBlock)")
)

// ToABI converts StateSyncEvent to ABI
func (sse *StateSyncedEvent) EncodeAbi() ([]byte, error) {
	return stateSyncABIType.Encode([]interface{}{sse})
}

var (
	_ StateTransactionInput = &CommitEpochValidatorSetFn{}
	_ StateTransactionInput = &DistributeRewardForRewardPoolFn{}
)

// IsStake indicates if transfer event (from ERC20 implementation) mints tokens to a non zero address
func (t *TransferEvent) IsStake() bool {
	return t.To != types.ZeroAddress && t.From == types.ZeroAddress
}

// IsUnstake indicates if transfer event (from ERC20 implementation) burns tokens from a non zero address
// meaning, it transfers them to zero address
func (t *TransferEvent) IsUnstake() bool {
	return t.To == types.ZeroAddress && t.From != types.ZeroAddress
}
