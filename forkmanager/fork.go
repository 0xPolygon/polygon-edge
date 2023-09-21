package forkmanager

import "github.com/0xPolygon/polygon-edge/helper/common"

const InitialFork = "initialfork"

// HandlerDesc gives description for the handler
// eq: "extra", "proposer_calculator", etc
type HandlerDesc string

// Fork structure defines one fork
type Fork struct {
	// name of the fork
	Name string
	// after the fork is activated, `FromBlockNumber` shows from which block is enabled
	FromBlockNumber uint64
	// fork consensus parameters
	Params *ForkParams
	// this value is false if fork is registered but not activated
	IsActive bool
	// map of all handlers registered for this fork
	Handlers map[HandlerDesc]interface{}
}

// ForkParams hard-coded fork params
type ForkParams struct {
	// MaxValidatorSetSize indicates the maximum size of validator set
	MaxValidatorSetSize *uint64 `json:"maxValidatorSetSize,omitempty"`

	// EpochSize is size of epoch
	EpochSize *uint64 `json:"epochSize,omitempty"`

	// SprintSize is size of sprint
	SprintSize *uint64 `json:"sprintSize,omitempty"`

	// BlockTime is target frequency of blocks production
	BlockTime *common.Duration `json:"blockTime,omitempty"`

	// BlockTimeDrift defines the time slot in which a new block can be created
	BlockTimeDrift *uint64 `json:"blockTimeDrift,omitempty"`
}

// Copy creates a deep copy of ForkParams
func (fp *ForkParams) Copy() *ForkParams {
	maxValSetSize := *fp.MaxValidatorSetSize
	epochSize := *fp.EpochSize
	sprintSize := *fp.SprintSize
	blockTime := *fp.BlockTime
	blockTimeDrift := *fp.BlockTimeDrift

	return &ForkParams{
		MaxValidatorSetSize: &maxValSetSize,
		EpochSize:           &epochSize,
		SprintSize:          &sprintSize,
		BlockTime:           &blockTime,
		BlockTimeDrift:      &blockTimeDrift,
	}
}

// forkHandler defines one custom handler
type forkHandler struct {
	// Handler should be active from block `FromBlockNumber``
	FromBlockNumber uint64
	// instance of some structure, function etc
	Handler interface{}
}

// forkParamsBlock encapsulates block and actual fork params
type forkParamsBlock struct {
	// Params should be active from block `FromBlockNumber``
	FromBlockNumber uint64
	// pointer to fork params
	Params *ForkParams
}
