package forkmanager

const InitialFork = "initialfork"

// Fork structure defines one fork
type Fork struct {
	// name of the fork
	Name string
	// after the fork is activated, `FromBlockNumber` shows from which block is enabled
	FromBlockNumber uint64
	// fork consensus parameters
	Params interface{}
	// this value is false if fork is registered but not activated
	IsActive bool
	// map of all handlers registered for this fork
	Handlers map[string]interface{}
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
	Params interface{}
}
