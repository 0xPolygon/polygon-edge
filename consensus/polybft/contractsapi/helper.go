package contractsapi

import "github.com/umbracle/ethgo/abi"

// specific case where we need to encode state sync event as a tuple of tuple
var stateSyncABIType = abi.MustNewType(
	"tuple(tuple(uint256 id, address sender, address receiver, bytes data))")

// ToABI converts StateSyncEvent to ABI
func (sse *StateSyncedEvent) EncodeAbi() ([]byte, error) {
	return stateSyncABIType.Encode([]interface{}{sse})
}
