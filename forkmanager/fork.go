package forkmanager

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/common"
)

const InitialFork = "initialfork"

// HandlerDesc gives description for the handler
// eq: "extra", "proposer_calculator", etc
type HandlerDesc string

// ForkParams hard-coded fork params
type ForkParams struct {
	// MaxValidatorSetSize indicates the maximum size of validator set
	MaxValidatorSetSize uint64 `json:"maxValidatorSetSize"`

	EpochSize uint64 `json:"epochSize"`

	// SprintSize is size of sprint
	SprintSize uint64 `json:"sprintSize"`

	// BlockTime is target frequency of blocks production
	BlockTime common.Duration `json:"blockTime"`

	// BlockTimeDrift defines the time slot in which a new block can be created
	BlockTimeDrift uint64 `json:"blockTimeDrift"`
}

// Fork structure defines one fork
type Fork struct {
	// name of the fork
	Name string
	// after the fork is activated, `FromBlockNumber` shows from which block is enabled
	FromBlockNumber uint64
	Params          *ForkParams
	// this value is false if fork is registered but not activated
	IsActive bool
	// map of all handlers registered for this fork
	Handlers map[HandlerDesc]interface{}
}

// Handler defines one custom handler
type Handler struct {
	// Handler should be active from block `FromBlockNumber``
	FromBlockNumber uint64
	// instance of some structure, function etc
	Handler interface{}
}

type ForkParamsBlock struct {
	// Params should be active from block `FromBlockNumber``
	FromBlockNumber uint64
	// actual params
	Params *ForkParams
}

func ForkManagerInit(factory func(*chain.Forks) error, forks *chain.Forks) error {
	if factory == nil {
		return nil
	}

	fm := GetInstance()
	fm.Clear()

	// register initial fork
	fm.RegisterFork(InitialFork, nil)

	var (
		forkParams *ForkParams
		ok         bool
	)

	// Register forks
	for name, f := range *forks {
		// check if fork is not supported by current edge version
		if _, ok = (*chain.AllForksEnabled)[name]; !ok {
			return fmt.Errorf("fork is not available: %s", name)
		}

		if dt := f.Params; dt != nil {
			customJSON, err := json.Marshal(dt)
			if err != nil {
				return fmt.Errorf("fork params error: %s, %w", name, err)
			}

			if err = json.Unmarshal(customJSON, &forkParams); err != nil {
				return fmt.Errorf("fork params error: %s, %w", name, err)
			}
		}

		fm.RegisterFork(name, forkParams)
	}

	// Register handlers and additional forks here
	if factory != nil {
		if err := factory(forks); err != nil {
			return err
		}
	}

	// Activate initial fork
	if err := fm.ActivateFork(InitialFork, uint64(0)); err != nil {
		return err
	}

	// Activate forks
	for name, f := range *forks {
		if f == nil {
			continue
		}

		if err := fm.ActivateFork(name, f.Block); err != nil {
			return err
		}
	}

	return nil
}
