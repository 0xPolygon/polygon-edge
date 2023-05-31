package forkmanager

// HandlerDesc gives description for the handler
// eq: "extra", "proposer_calculator", etc
type HandlerDesc string

// Fork structure defines one fork
type Fork struct {
	// name of the fork
	Name string
	// after the fork is activated, `FromBlockNumber` shows from which block is enabled
	FromBlockNumber uint64
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
