package forkmanager

type ForkHandlerName string

type ForkName string

type ForkInfo struct {
	Name            ForkName
	FromBlockNumber uint64
}

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

type ForkHandler struct {
	ForkName    ForkName
	HandlerName ForkHandlerName
	Handler     interface{}
}

func NewForkInfo(name ForkName, blockNumber uint64) *ForkInfo {
	return &ForkInfo{
		Name:            name,
		FromBlockNumber: blockNumber,
	}
}

func NewForkHandler(name ForkName, handlerName ForkHandlerName, handler interface{}) ForkHandler {
	return ForkHandler{
		ForkName:    name,
		HandlerName: handlerName,
		Handler:     handler,
	}
}
