package ibft

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/helper/common"
)

// Define the type of the IBFT consensus

type MechanismType string

const (
	// PoA defines the Proof of Authority IBFT type,
	// where the validator set is changed through voting / pre-set in genesis
	PoA MechanismType = "PoA"

	// PoS defines the Proof of Stake IBFT type,
	// where the validator set it changed through staking on the Staking SC
	PoS MechanismType = "PoS"
)

// mechanismTypes is the map used for easy string -> mechanism MechanismType lookups
var mechanismTypes = map[string]MechanismType{
	"PoA": PoA,
	"PoS": PoS,
}

// String is a helper method for casting a MechanismType to a string representation
func (t MechanismType) String() string {
	return string(t)
}

// ParseType converts a mechanism string representation to a MechanismType
func ParseType(mechanism string) (MechanismType, error) {
	// Check if the cast is possible
	castType, ok := mechanismTypes[mechanism]
	if !ok {
		return castType, fmt.Errorf("invalid IBFT mechanism type %s", mechanism)
	}

	return castType, nil
}

// Define the type of Hook

type HookType string

const (
	// POA //

	// VerifyHeadersHook defines additional checks that need to happen
	// when verifying the headers
	VerifyHeadersHook HookType = "VerifyHeadersHook"

	// ProcessHeadersHook defines additional steps that need to happen
	// when processing the headers
	ProcessHeadersHook HookType = "ProcessHeadersHook"

	// InsertBlockHook defines additional steps that need to happen
	// when inserting a block into the chain
	InsertBlockHook HookType = "InsertBlockHook"

	// CandidateVoteHook defines additional steps that need to happen
	// when building a block (candidate voting)
	CandidateVoteHook HookType = "CandidateVoteHook"

	// POA + POS //

	// AcceptStateLogHook defines what should be logged out as the status
	// from AcceptState
	AcceptStateLogHook HookType = "AcceptStateLogHook"

	// POS //

	// VerifyBlockHook defines the additional verification steps for the PoS mechanism
	VerifyBlockHook HookType = "VerifyBlockHook"

	// PreStateCommitHook defines the additional state transition injection
	PreStateCommitHook HookType = "PreStateCommitHook"

	// CalculateProposerHook defines what is the next proposer
	// based on the previous
	CalculateProposerHook = "CalculateProposerHook"
)

type ConsensusMechanism interface {
	// GetType returns the type of IBFT consensus mechanism (PoA / PoS)
	GetType() MechanismType

	// GetHookMap returns the hooks registered with the specific consensus mechanism
	GetHookMap() map[HookType]func(interface{}) error

	// IsAvailable returns whether the corresponding hook is available
	IsAvailable(hook HookType, height uint64) bool

	// ShouldWriteTransactions returns whether transactions should be written to a block
	// from the TxPool
	ShouldWriteTransactions(blockNumber uint64) bool

	// initializeHookMap initializes the hook map
	initializeHookMap()
}

type BaseConsensusMechanism struct {
	// Reference to the main IBFT implementation
	ibft *Ibft

	// hookMap is the collection of registered hooks
	hookMap map[HookType]func(interface{}) error

	// Used for easy lookups
	mechanismType MechanismType

	// Available periods
	From uint64
	To   *uint64
}

// initializeParams initializes mechanism parameters from chain config
func (base *BaseConsensusMechanism) initializeParams(params *IBFTFork) error {
	if params == nil {
		return nil
	}

	base.From = params.From.Value

	if params.To != nil {
		if params.To.Value < base.From {
			return fmt.Errorf(
				`"to" must be grater than or equal to from: from=%d, to=%d`,
				base.From,
				params.To.Value,
			)
		}

		base.To = &params.To.Value
	}

	return nil
}

// GetType implements the ConsensusMechanism interface method
func (base *BaseConsensusMechanism) GetType() MechanismType {
	return base.mechanismType
}

// GetHookMap implements the ConsensusMechanism interface method
func (base *BaseConsensusMechanism) GetHookMap() map[HookType]func(interface{}) error {
	return base.hookMap
}

// IsInRange returns indicates if the given blockNumber is between from and to
func (base *BaseConsensusMechanism) IsInRange(blockNumber uint64) bool {
	// not ready
	if blockNumber < base.From {
		return false
	}

	// expired
	if base.To != nil && *base.To < blockNumber {
		return false
	}

	return true
}

// IBFT Fork represents setting in params.engine.ibft of genesis.json
type IBFTFork struct {
	Type              MechanismType      `json:"type"`
	Deployment        *common.JSONNumber `json:"deployment,omitempty"`
	From              common.JSONNumber  `json:"from"`
	To                *common.JSONNumber `json:"to,omitempty"`
	MaxValidatorCount *common.JSONNumber `json:"maxValidatorCount,omitempty"`
	MinValidatorCount *common.JSONNumber `json:"minValidatorCount,omitempty"`
}

// ConsensusMechanismFactory is the factory function to create a consensus mechanism
type ConsensusMechanismFactory func(ibft *Ibft, params *IBFTFork) (ConsensusMechanism, error)

var mechanismBackends = map[MechanismType]ConsensusMechanismFactory{
	PoA: PoAFactory,
	PoS: PoSFactory,
}
