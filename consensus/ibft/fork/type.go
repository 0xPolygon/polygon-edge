package fork

import "fmt"

// Define the type of the IBFT consensus
type IBFTType string

const (
	// PoA defines the Proof of Authority IBFT type,
	// where the validator set is changed through voting / pre-set in genesis
	PoA IBFTType = "PoA"

	// PoS defines the Proof of Stake IBFT type,
	// where the validator set it changed through staking on the Staking SC
	PoS IBFTType = "PoS"
)

// mechanismTypes is the map used for easy string -> mechanism MechanismType lookups
var ibftTypes = map[string]IBFTType{
	"PoA": PoA,
	"PoS": PoS,
}

// String is a helper method for casting a MechanismType to a string representation
func (t IBFTType) String() string {
	return string(t)
}

// ParseType converts a mechanism string representation to a MechanismType
func ParseIBFTType(ibftType string) (IBFTType, error) {
	// Check if the cast is possible
	castType, ok := ibftTypes[ibftType]
	if !ok {
		return castType, fmt.Errorf("invalid IBFT type %s", ibftType)
	}

	return castType, nil
}
