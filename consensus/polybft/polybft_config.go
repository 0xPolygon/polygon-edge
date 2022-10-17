package polybft

import (
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
)

// PolyBFTConfig is the configuration file for the Polybft consensus protocol.
type PolyBFTConfig struct {
	InitialValidatorSet []*Validator  `json:"initialValidatorSet"`
	Bridge              *BridgeConfig `json:"bridge"`

	ValidatorSetSize int `json:"validatorSetSize"`

	// Address of the system contracts, as of now (testing) this is populated automatically during genesis
	ValidatorSetAddr  types.Address `json:"validatorSetAddr"`
	StateReceiverAddr types.Address `json:"stateReceiverAddr"`

	// size of the epoch and sprint
	EpochSize  uint64 `json:"epochSize"`
	SprintSize uint64 `json:"sprintSize"`

	BlockTime time.Duration `json:"blockTime"`

	// Governance is the initial governance address
	Governance types.Address `json:"governance"`
}

type SmartContract struct {
	Address types.Address `json:"address"`
	Code    []byte        `json:"code"`
	Name    string        `json:"name"`
}

// BridgeConfig is the configuration for the bridge
type BridgeConfig struct {
	BridgeAddr      types.Address `json:"bridgeAddr"`
	CheckpointAddr  types.Address `json:"checkpointAddr"`
	JSONRPCEndpoint string        `json:"jsonRPCEndpoint"`
}

func (p *PolyBFTConfig) IsBridgeEnabled() bool {
	return p.Bridge != nil
}

type Validator struct {
	Address types.Address
	BlsKey  string
	Balance *big.Int `json:"balance"`
}

// DebugConfig is a struct used for test configuration in init genesis
type DebugConfig struct {
	ValidatorSetSize uint64 `json:"validatorSetSize"`
}

// PolyBFTRegisterMessage is a challenge message which needs to be signed by each validator account
var PolyBFTRegisterMessage = "Polybft validator"
