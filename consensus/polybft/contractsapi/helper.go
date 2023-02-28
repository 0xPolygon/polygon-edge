package contractsapi

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

// StateTransactionInput is an abstraction for different state transaction inputs
type StateTransactionInput interface {
	// EncodeAbi contains logic for encoding arbitrary data into ABI format
	EncodeAbi() ([]byte, error)
	// DecodeAbi contains logic for decoding given ABI data
	DecodeAbi(b []byte) error
}

var (
	// specific case where we need to encode state sync event as a tuple of tuple
	stateSyncABIType = abi.MustNewType(
		"tuple(tuple(uint256 id, address sender, address receiver, bytes data))")
)

// ToABI converts StateSyncEvent to ABI
func (sse *StateSyncedEvent) EncodeAbi() ([]byte, error) {
	return stateSyncABIType.Encode([]interface{}{sse})
}

// AddValidatorUptime is an extension (helper) function on a generated Uptime type
// that adds uptime data for given validator to Uptime struct
func (u *Uptime) AddValidatorUptime(address types.Address, count int64) {
	u.UptimeData = append(u.UptimeData, &UptimeData{
		Validator:    address,
		SignedBlocks: big.NewInt(count),
	})
}

var _ StateTransactionInput = &CommitEpochFunction{}
