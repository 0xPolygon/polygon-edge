package forkmanager

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
)

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
	Params *chain.ForkParams
	// this value is false if fork is registered but not activated
	IsActive bool
	// map of all handlers registered for this fork
	Handlers map[HandlerDesc]interface{}
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
	Params *chain.ForkParams
}

func ForkManagerInit(
	initialParams *chain.ForkParams,
	factory func(*chain.Forks) error,
	forks *chain.Forks) error {
	if factory == nil {
		return nil
	}

	fm := GetInstance()
	fm.Clear()

	// register initial fork
	fm.RegisterFork(InitialFork, initialParams)

	// Register forks
	for name, f := range *forks {
		// check if fork is not supported by current edge version
		if _, found := (*chain.AllForksEnabled)[name]; !found {
			return fmt.Errorf("fork is not available: %s", name)
		}

		fm.RegisterFork(name, f.Params)
	}

	// Register handlers and additional forks here
	if err := factory(forks); err != nil {
		return err
	}

	// Activate initial fork
	if err := fm.ActivateFork(InitialFork, uint64(0)); err != nil {
		return err
	}

	// Activate forks
	for name, f := range *forks {
		if err := fm.ActivateFork(name, f.Block); err != nil {
			return err
		}
	}

	return nil
}
