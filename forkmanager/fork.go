package forkmanager

type ForkHandlerName string

type ForkName string

type Fork struct {
	Name            ForkName
	FromBlockNumber uint64
	IsActive        bool
	Handlers        map[ForkHandlerName]interface{}
}

type ForkActiveHandler struct {
	FromBlockNumber uint64
	Handler         interface{}
}
