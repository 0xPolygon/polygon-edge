package fork

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/validators/store"
)

// Define the type of the IBFT consensus
type IBFTType string

const (
	// PoA defines the Proof of Authority IBFT type,
	// where the validator set is changed through voting / pre-set in genesis
	PoA IBFTType = "PoA"

	// PoS defines the Proof of Stake IBFT type,
	// where the validator set it changed through staking on the Staking Smart Contract
	PoS IBFTType = "PoS"
)

// ibftTypes is the map used for easy string -> IBFTType lookups
var ibftTypes = map[string]IBFTType{
	"PoA": PoA,
	"PoS": PoS,
}

// ibftTypesToSourceType defines validator set type used under each IBFT Type
// Right now each IBFT Type is correspond one-to-one with ValidatorStore
// In other words, PoA always uses SnapshotValidatorStore while PoS uses ContractValidatorStore
// By definition, PoA can fetch validators from ContractValidatorStore
var ibftTypesToSourceType = map[IBFTType]store.SourceType{
	PoA: store.Snapshot,
	PoS: store.Contract,
}

// String is a helper method for casting a IBFTType to a string representation
func (t IBFTType) String() string {
	return string(t)
}

// ParseIBFTType converts a ibftType string representation to a IBFTType
func ParseIBFTType(ibftType string) (IBFTType, error) {
	// Check if the cast is possible
	castType, ok := ibftTypes[ibftType]
	if !ok {
		return castType, fmt.Errorf("invalid IBFT type %s", ibftType)
	}

	return castType, nil
}
